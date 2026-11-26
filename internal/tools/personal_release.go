package tools

import (
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
)

// How a person's session stops being offered: its call returns, its
// connection dies, its credential changes, it sits idle, or its server goes.

// release ends one call on the slot, and closes it if it was retired and this
// was its last.
func (p *personalPool) release(slot *personalSlot) {
	p.mu.Lock()
	slot.busy--
	slot.lastUsed = p.now()
	done := slot.retired && slot.busy == 0
	p.signalLocked()
	p.mu.Unlock()
	if done {
		// Already counted in draining, since retireLocked saw it busy.
		p.closeCounted([]Session{slot.session})
	}
}

// closeCounted closes sessions counted in draining, and only once each close
// has returned gives their places back: until then the connection is open.
func (p *personalPool) closeCounted(sessions []Session) {
	if len(sessions) == 0 {
		return
	}
	closeAll(sessions)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.draining -= len(sessions)
	p.signalLocked()
}

// retire stops offering the slot when err says its connection is gone. A
// tool's own error is an answer, and the session that gave it stays.
func (p *personalPool) retire(who domain.UserID, slot *personalSlot, err error) {
	if !errors.Is(err, mcp.ErrConnectionClosed) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retireLocked(who, slot)
}

// retireLocked stops offering the slot. One with a call on it keeps its place
// against the limit until release closes it.
func (p *personalPool) retireLocked(who domain.UserID, slot *personalSlot) {
	if slot.retired {
		return
	}
	if p.slots[who] == slot {
		delete(p.slots, who)
	}
	slot.retired = true
	if slot.busy > 0 {
		p.draining++
	}
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
		p.retireLocked(who, slot)
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
