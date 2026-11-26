package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
)

// PersonalOpener opens a session on one server for one person: its initialize,
// and every request after it, carry that person's credential.
type PersonalOpener func(ctx context.Context, principal domain.UserID) (Session, error)

/*
PersonalSessions serves a server's calls from a session per person.

For a server that takes each person's own credential. One session for
everybody, initialized with nobody's, only changed the header of each request:
a server that knows who is calling by the token it saw at initialize answered
everyone as nobody. Each person's session is opened on their first call, kept
while they use it, and closed when it has sat idle, or to make room.
*/
type PersonalSessions struct {
	Open PersonalOpener
	// Identity is who the person's credential speaks as now (see
	// domain.MCPCredentials.SessionIdentity). A session opened under another
	// is retired and the call goes to a new one. Nil is one identity per
	// person.
	Identity func(ctx context.Context, principal domain.UserID) (string, error)
	// Limit is how many may be open at once for the server, on this worker.
	// Zero is DefaultPersonalSessionLimit.
	Limit int
	// Idle is how long one may sit unused before the next call closes it.
	// Zero is DefaultPersonalSessionIdle.
	Idle time.Duration
}

const (
	DefaultPersonalSessionLimit = 32
	DefaultPersonalSessionIdle  = 10 * time.Minute
)

// WithPersonalSessions serves the server's calls on behalf of a person from
// that person's own session. A nil opener is the shared session alone.
func WithPersonalSessions(sessions PersonalSessions) ServerOption {
	return func(options *serverOptions) {
		if sessions.Open == nil {
			options.personal = nil
			return
		}
		options.personal = &sessions
	}
}

// errPersonalSessionsClosed is a call that arrived as its server went away.
var errPersonalSessionsClosed = errors.New("tools: the server's personal sessions are closed")

type personalPool struct {
	open     PersonalOpener
	identity func(ctx context.Context, principal domain.UserID) (string, error)
	limit    int
	idle     time.Duration
	now      func() time.Time

	mu      sync.Mutex
	slots   map[domain.UserID]*personalSlot
	opening map[domain.UserID]chan struct{}
	// changed is closed, and replaced, whenever a place may have come free.
	changed chan struct{}
	closed  bool
}

type personalSlot struct {
	session  Session
	identity string
	busy     int
	lastUsed time.Time
	// retired is a slot no longer offered, closed when its last call returns.
	retired bool
}

func newPersonalPool(sessions PersonalSessions, now func() time.Time) *personalPool {
	limit, idle := sessions.Limit, sessions.Idle
	if limit <= 0 {
		limit = DefaultPersonalSessionLimit
	}
	if idle <= 0 {
		idle = DefaultPersonalSessionIdle
	}
	return &personalPool{
		open: sessions.Open, identity: sessions.Identity, limit: limit, idle: idle, now: now,
		slots:   make(map[domain.UserID]*personalSlot),
		opening: make(map[domain.UserID]chan struct{}),
		changed: make(chan struct{}),
	}
}

/*
acquire is the person's session, opened if they have none, with a call
counted on it until release.

A session is offered only once its open has returned: two first calls of one
person wait for the same open, and one that fails leaves no slot behind. At the
limit, the least recently used idle session is closed to make room; when every
one has a call in flight, the call waits for a place, and gives up when its
context does.
*/
func (p *personalPool) acquire(ctx context.Context, who domain.UserID) (*personalSlot, error) {
	identity, err := p.identityOf(ctx, who)
	if err != nil {
		return nil, fmt.Errorf("read %s's credential: %w", who, err)
	}
	for {
		slot, wait, stale, err := p.next(who, identity)
		closeAll(stale)
		switch {
		case err != nil:
			return nil, err
		case slot != nil:
			return slot, nil
		case wait != nil:
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return p.opened(ctx, who, identity)
	}
}

func (p *personalPool) identityOf(ctx context.Context, who domain.UserID) (string, error) {
	if p.identity == nil {
		return "", nil
	}
	return p.identity(ctx, who)
}

// next decides, under the lock, what acquire does: use a slot, wait, or open
// (all three nil). stale are the sessions to close once the lock is let go.
func (p *personalPool) next(who domain.UserID, identity string) (*personalSlot, <-chan struct{}, []Session, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, nil, nil, errPersonalSessionsClosed
	}
	now := p.now()
	stale := p.expireLocked(now)
	if slot := p.slots[who]; slot != nil && slot.identity != identity {
		// Opened as somebody the credential no longer speaks for: retired,
		// and closed now if no call is still on it.
		delete(p.slots, who)
		slot.retired = true
		if slot.busy == 0 {
			stale = append(stale, slot.session)
		}
	}
	if slot := p.slots[who]; slot != nil {
		slot.busy++
		slot.lastUsed = now
		return slot, nil, stale, nil
	}
	if opening, ok := p.opening[who]; ok {
		return nil, opening, stale, nil
	}
	if len(p.slots)+len(p.opening) >= p.limit {
		evicted, ok := p.evictLocked()
		if !ok {
			return nil, p.changed, stale, nil
		}
		stale = append(stale, evicted)
	}
	p.opening[who] = make(chan struct{})
	return nil, nil, stale, nil
}

func (p *personalPool) opened(ctx context.Context, who domain.UserID, identity string) (*personalSlot, error) {
	session, err := p.open(ctx, who)
	p.mu.Lock()
	defer p.mu.Unlock()
	close(p.opening[who])
	delete(p.opening, who)
	p.signalLocked()
	if err != nil {
		return nil, err
	}
	if p.closed {
		_ = session.Close()
		return nil, errPersonalSessionsClosed
	}
	slot := &personalSlot{session: session, identity: identity, busy: 1, lastUsed: p.now()}
	p.slots[who] = slot
	return slot, nil
}

// release ends one call on the slot.
func (p *personalPool) release(slot *personalSlot) {
	p.mu.Lock()
	slot.busy--
	slot.lastUsed = p.now()
	done := slot.retired && slot.busy == 0
	p.signalLocked()
	p.mu.Unlock()
	if done {
		_ = slot.session.Close()
	}
}

// retire stops offering the slot when err says its connection is gone. A
// tool's own error is an answer, and the session that gave it stays.
func (p *personalPool) retire(who domain.UserID, slot *personalSlot, err error) {
	if !errors.Is(err, mcp.ErrConnectionClosed) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.slots[who] == slot {
		delete(p.slots, who)
	}
	slot.retired = true
}

// detach stops the pool as its server goes and answers the idle sessions to
// close; a busy one closes when its last call returns. The closes are I/O, left
// to the caller to make once it holds no lock other servers' calls wait on.
func (p *personalPool) detach() []Session {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	var idle []Session
	for who, slot := range p.slots {
		delete(p.slots, who)
		slot.retired = true
		if slot.busy == 0 {
			idle = append(idle, slot.session)
		}
	}
	p.signalLocked()
	return idle
}

func (p *personalPool) expireLocked(now time.Time) []Session {
	var stale []Session
	for who, slot := range p.slots {
		if slot.busy == 0 && now.Sub(slot.lastUsed) > p.idle {
			delete(p.slots, who)
			stale = append(stale, slot.session)
		}
	}
	return stale
}

// evictLocked takes the least recently used slot with no call in flight.
func (p *personalPool) evictLocked() (Session, bool) {
	var (
		oldest *personalSlot
		owner  domain.UserID
	)
	for who, slot := range p.slots {
		if slot.busy == 0 && (oldest == nil || slot.lastUsed.Before(oldest.lastUsed)) {
			oldest, owner = slot, who
		}
	}
	if oldest == nil {
		return nil, false
	}
	delete(p.slots, owner)
	return oldest.session, true
}

func (p *personalPool) signalLocked() {
	close(p.changed)
	p.changed = make(chan struct{})
}

func closeAll(sessions []Session) {
	for _, s := range sessions {
		_ = s.Close()
	}
}
