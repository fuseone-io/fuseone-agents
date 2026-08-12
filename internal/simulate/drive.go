package simulate

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/engine"
)

// Advancer is one turn of a run, declared here by the consumer.
type Advancer interface {
	Advance(ctx context.Context, start engine.Start) (engine.Status, error)
}

/*
Drive turns a case into a finished simulation.

It stops where a real run would stop, including at an approval. Waiting for a
person is a result rather than a pause to wait out: "it would have asked you
here" is the sentence a simulation exists to produce, and answering on the
author's behalf would invent the one decision this product refuses to make for
anybody.

The turn bound is not the budget. A run bounded only by money would spend the
whole ceiling of every case in a set of fifty before anybody saw a report, and
a planner that will not finish is exactly the thing a simulation is meant to
reveal — cheaply, and once per case.
*/
func Drive(
	ctx context.Context, adv Advancer, start engine.Start, maxTurns int,
) (engine.Status, error) {
	var status engine.Status
	for turn := 0; turn < maxTurns; turn++ {
		got, err := adv.Advance(ctx, start)
		if err != nil {
			return status, err
		}
		status = got

		switch status.Phase {
		case engine.PhaseFinished, engine.PhaseParked, engine.PhaseAwaitingApproval:
			return status, nil
		}
	}
	return status, fmt.Errorf("simulate: the run did not settle in %d turns", maxTurns)
}
