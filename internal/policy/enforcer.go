package policy

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

// Enforcer holds the set in force and hands the Gate a snapshot of it.
//
// The split is deliberate. The Gate takes a snapshot and returns a decision —
// it is on the path of every effect and must not depend on a query succeeding
// to say whether something is allowed. This owns freshness instead: it reads
// the set on a timer and swaps it, so a database that is briefly unreachable
// means decisions keep being made under the last set anybody saw, and the hash
// on every step says which one that was.
type Enforcer struct {
	source Source
	log    *slog.Logger

	mu      sync.RWMutex
	current *gate.Gate
	hash    string
}

// Source is where the set comes from, declared here by the consumer.
type Source interface {
	Active(ctx context.Context) (Set, error)
}

// NewEnforcer starts from the built-in ladder alone. Until the first refresh
// lands, decisions are made under the safe default rather than under nothing.
func NewEnforcer(source Source, log *slog.Logger) *Enforcer {
	return &Enforcer{source: source, log: log, current: gate.New()}
}

// Evaluate answers from whichever snapshot is current.
func (e *Enforcer) Evaluate(ctx context.Context, r gate.Request) (domain.Decision, error) {
	e.mu.RLock()
	g := e.current
	e.mu.RUnlock()
	return g.Evaluate(ctx, r)
}

// Refresh reads the set and swaps it in.
func (e *Enforcer) Refresh(ctx context.Context) error {
	set, err := e.source.Active(ctx)
	if err != nil {
		return fmt.Errorf("policy: refresh: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if set.Hash == e.hash {
		return nil
	}
	e.current, e.hash = gate.New().WithPolicies(gate.Policies{Hash: set.Hash, Set: set.Policies}), set.Hash
	e.log.Info("policy set in force", "hash", set.Hash, "policies", len(set.Policies))
	return nil
}

// Hash is the set currently deciding. Reported so an operator can tell whether
// a worker has picked up a change yet.
func (e *Enforcer) Hash() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.hash
}

// Watch refreshes until the context ends.
//
// A failed refresh is logged and not fatal: the last set keeps deciding, which
// is the behaviour worth having when the alternative is a worker that stops
// enforcing anything because a query timed out.
func (e *Enforcer) Watch(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.Refresh(ctx); err != nil && ctx.Err() == nil {
				e.log.Error("could not refresh the policy set; the last one keeps deciding",
					"hash", e.Hash(), "err", err)
			}
		}
	}
}
