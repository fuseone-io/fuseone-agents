package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// PersonalOpener opens a session on one server for one person, as identity:
// its initialize, and every request after it, carry that person's credential,
// and are refused with ErrSessionIdentityChanged once it speaks as another.
type PersonalOpener func(ctx context.Context, principal domain.UserID, identity string) (Session, error)

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
	// draining are sessions no longer offered that are still open: a call
	// still on them, or a close not yet returned. They count against the
	// limit until closed.
	draining int
	closed   bool
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
		p.closeCounted(stale)
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
func (p *personalPool) next(who domain.UserID, identity string) (slot *personalSlot, wait <-chan struct{}, stale []Session, err error) {
	p.mu.Lock()
	defer func() {
		// Each one keeps its place until closeCounted has closed it.
		p.draining += len(stale)
		p.mu.Unlock()
	}()
	if p.closed {
		return nil, nil, nil, errPersonalSessionsClosed
	}
	now := p.now()
	stale = p.expireLocked(now)
	if slot := p.slots[who]; slot != nil && slot.identity != identity {
		// Opened as somebody the credential no longer speaks for: retired,
		// and closed now if no call is still on it.
		p.retireLocked(who, slot)
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
	if len(p.slots)+len(p.opening)+p.draining >= p.limit {
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
	session, err := p.open(ctx, who, identity)
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

func (p *personalPool) signalLocked() {
	close(p.changed)
	p.changed = make(chan struct{})
}

func closeAll(sessions []Session) {
	for _, s := range sessions {
		_ = s.Close()
	}
}
