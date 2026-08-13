// Undoing: what happens to a run a person ended.
//
// Separate from the loop because it is not a turn. The loop asks the model
// what to do next; this carries out a decision already made, and the only
// thing it can discover is that something could not be taken back.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/fuseone/agents/internal/compensate"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
undo performs the compensation of an abandoned run and then ends it.

It runs here, on a worker, rather than in the request that asked for it: the
undos are real tool calls against real systems, and one of them can take
minutes. The person gets the run back immediately in the compensating phase and
watches it over the same stream as any other run.

The run ends failed whatever happened. An undo that did not work leaves
something standing in the world, and the answer to that is a person reading the
trail — not a worker retrying a financial reversal until it sticks.
*/
func (w *Worker) undo(ctx context.Context, claim domain.Claim, start engine.Start, log *slog.Logger) error {
	steps, err := w.deps.Ledger.Read(ctx, claim.RunID, 0)
	if err != nil {
		return w.park(ctx, claim, err, "trail_unreadable")
	}

	plan := compensate.Plan(steps, w.compensators())
	outcomes, err := compensate.Perform(ctx, compensate.Deps{
		Ledger: w.deps.Ledger, Gate: w.deps.Gate, Tools: w.deps.Tools,
		Catalog: w.deps.Catalog, Clock: w.deps.Clock, Content: w.deps.Content,
	}, start, plan)
	if err != nil {
		// The ledger or the Gate is broken, not the undoing. Park so the run
		// keeps its compensating phase and a later turn finishes the job —
		// the steps already recorded stop it repeating what worked.
		return w.park(ctx, claim, err, "compensation_interrupted")
	}

	standing := 0
	for _, o := range outcomes {
		if !o.Done {
			standing++
		}
	}
	log.Info("compensated", "acts", len(outcomes), "standing", standing)

	return w.end(ctx, claim, standing)
}

// end closes an abandoned run, saying how much of the world it could not
// return to how it found it.
func (w *Worker) end(ctx context.Context, claim domain.Claim, standing int) error {
	payload, err := json.Marshal(domain.FailedPayload{
		Code: "abandoned",
		Message: fmt.Sprintf(
			"abandoned by a person; %d act(s) could not be undone", standing),
	})
	if err != nil {
		return fmt.Errorf("encode ending of %s: %w", claim.RunID, err)
	}
	if _, err := w.deps.Ledger.Append(ctx, domain.Step{
		RunID: claim.RunID, Kind: domain.StepFailed, Scope: claim.Scope,
		AgentID: claim.AgentID, VersionID: claim.VersionID,
		OnBehalfOf: claim.OnBehalfOf, At: w.deps.Clock.Now(), Payload: payload,
	}); err != nil {
		return fmt.Errorf("end %s: %w", claim.RunID, err)
	}
	return w.release(ctx, claim, domain.ClaimOutcome{})
}

// compensators is the catalogue's view of what undoes what. An installation
// that has ruled on nothing gets an empty one, and every act comes back
// reported as standing rather than silently dropped.
func (w *Worker) compensators() compensate.Catalogue {
	if w.undos == nil {
		return noUndos{}
	}
	return w.undos
}

type noUndos struct{}

func (noUndos) CompensatedBy(domain.ToolID) (domain.ToolID, bool) { return "", false }
