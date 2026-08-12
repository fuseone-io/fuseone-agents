package ledger

import (
	"context"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// simulated is the half of the queue holding the runs a simulation opened.
type simulated interface {
	claimSimulated(ctx context.Context, owner string, lease time.Duration) (domain.Claim, error)
	Release(ctx context.Context, runID domain.RunID, outcome domain.ClaimOutcome) error
}

/*
SimulationQueue is the same queue, restricted to the runs a simulation opened.

A queue of its own rather than a flag on the existing one. The pool that drains
this is built with the dry tool layer and the pool that drains the other with
the real one; two queues cannot hand a simulated run to a worker holding real
tools, and one queue with a parameter can — that is a single mistaken argument
away from executing a dry run's proposals against production, on the strength
of a case somebody uploaded to find out what would happen.

Everything else is shared: the lease, the backoff, the attempt count and the
parking. A simulated run is a run.
*/
type SimulationQueue struct{ src simulated }

// Simulations returns the half of this ledger's queue holding simulated runs.
func (p *Postgres) Simulations() SimulationQueue { return SimulationQueue{p} }

// Simulations returns the half of this ledger's queue holding simulated runs.
func (m *Memory) Simulations() SimulationQueue { return SimulationQueue{m} }

func (q SimulationQueue) Claim(
	ctx context.Context, owner string, lease time.Duration,
) (domain.Claim, error) {
	return q.src.claimSimulated(ctx, owner, lease)
}

func (q SimulationQueue) Release(
	ctx context.Context, runID domain.RunID, outcome domain.ClaimOutcome,
) error {
	return q.src.Release(ctx, runID, outcome)
}
