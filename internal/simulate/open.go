package simulate

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/trigger"
)

// Opener opens a run, declared here by the consumer.
//
// The same opener every other trigger uses. A simulated run is a real run with
// one thing missing, and a second path that "just appends run_started" is how
// the mark, the pinned version or the idempotency key gets forgotten on one of
// them.
type Opener interface {
	Open(ctx context.Context, req trigger.Request) (trigger.Result, error)
}

// Batch is one simulation: an agent, and the occurrences to replay against it.
type Batch struct {
	// ID names the simulation. Generated at the edge and recorded in every
	// run it opens, so the report can find them again.
	ID    string
	Agent domain.AgentID
	// By is who asked. A simulation spends real money and the trail says whose
	// decision that was.
	By    domain.UserID
	Cases [][]byte
}

// Opened is what a batch produced.
type Opened struct {
	Runs []domain.RunID
	// Failed holds the reason each case that never opened gave. A set of fifty
	// that produced forty-eight runs is a set of forty-eight cases, and an
	// author told only the forty-eight has coverage that is a lie.
	Failed []string
}

/*
Open opens one simulated run per case, and drives nothing.

The runs are the queue. A simulated run is claimed by the pool built with the
dry tool layer and advanced turn by turn like every other run — same lease,
same backoff, same parking, same step ceiling. So there is no second queue to
keep in step with the first, and no moment at which a simulation is a kind of
work the platform handles specially.

Opening is quick: an append per case, no model call, nothing to wait for. That
is why it happens inside the request that asked for it, and why the number
answered is the number that actually opened.
*/
func Open(ctx context.Context, opener Opener, batch Batch) (Opened, error) {
	var out Opened
	for i, input := range batch.Cases {
		if err := ctx.Err(); err != nil {
			return out, err
		}

		opened, err := opener.Open(ctx, trigger.Request{
			Agent: batch.Agent,
			// The intention is this case of this simulation. Simulating again
			// is a new intention and opens its own runs; retrying one that
			// timed out reaches the runs it already opened.
			IdemKey:    fmt.Sprintf("sim:%s:%d", batch.ID, i+1),
			Trigger:    "simulation",
			By:         batch.By,
			Input:      input,
			Simulation: batch.ID,
		})
		if err != nil {
			// One case failing never stops the rest.
			out.Failed = append(out.Failed, fmt.Sprintf("case %d: %v", i+1, err))
			continue
		}
		out.Runs = append(out.Runs, opened.RunID)
	}
	return out, nil
}
