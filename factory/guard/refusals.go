package guard

import (
	"sync"
	"time"
)

// refusalTTL is how long a merge_if_ready refusal can be sent back. A run
// sends back what it was just told, within the same turn or the next.
const refusalTTL = time.Hour

// maxRefusals bounds the log: past it, expired entries go first, then all.
const maxRefusals = 256

type refusalKey struct {
	pr     int
	head   string
	reason Reason
}

// refusals is the server's own record of the merges GitHub refused or whose
// outcome was lost. Those two causes cannot be rebuilt by reading GitHub
// afterwards, so send_back accepts them only when this process observed
// them; a caller's word is never proof. The record is this process's memory
// and nothing else: a run resumed on another worker finds none, send_back
// refuses, and the next run proposes the merge again.
type refusals struct {
	clock Clock

	mu   sync.Mutex
	seen map[refusalKey]time.Time
}

func newRefusals(clock Clock) *refusals {
	if clock == nil {
		clock = SystemClock{}
	}
	return &refusals{clock: clock, seen: map[refusalKey]time.Time{}}
}

func (r *refusals) record(pr int, head string, reason Reason) {
	if r == nil {
		return
	}
	now := r.clock.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.seen) >= maxRefusals {
		for k, at := range r.seen {
			if now.Sub(at) >= refusalTTL {
				delete(r.seen, k)
			}
		}
		if len(r.seen) >= maxRefusals {
			clear(r.seen)
		}
	}
	r.seen[refusalKey{pr, head, reason}] = now
}

func (r *refusals) observed(pr int, head string, reason Reason) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	at, ok := r.seen[refusalKey{pr, head, reason}]
	r.mu.Unlock()
	return ok && r.clock.Now().Sub(at) < refusalTTL
}

// unrebuildable is a cause only the merge itself can show: GitHub refused
// it, or its answer was lost. Every other cause is read again from GitHub.
func unrebuildable(cause Reason) bool {
	return cause == ReasonMergeRefused || cause == ReasonOutcomeUnknown
}
