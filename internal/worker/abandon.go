// Undoing: what happens to a run a person ended.
//
// Separate from the loop because it is not a turn. The loop asks the model
// what to do next; this carries out a decision already made, and the only
// thing it can discover is that something could not be taken back.
package worker

import (
	"context"
	"encoding/json"
	"errors"
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
	if errors.Is(err, domain.ErrLeaseLost) {
		// Fenced mid-compensation: the successor runs the same plan, and the
		// steps already recorded stop it repeating what worked here.
		log.Warn("lease lost mid-compensation; leaving the run to its successor")
		return nil
	}
	if err != nil {
		// The ledger or the Gate is broken, not the undoing. Park so the run
		// keeps its compensating phase and a later turn finishes the job —
		// the steps already recorded stop it repeating what worked.
		return w.park(ctx, claim, err, "compensation_interrupted")
	}

	standing := 0
	unknown := 0
	for _, o := range outcomes {
		if !o.Done {
			standing++
		}
		if !o.Done && o.Act.Attempted && !o.Act.Failed {
			unknown++
		}
	}
	log.Info("compensated", "acts", len(outcomes), "standing", standing, "unknown", unknown)

	return w.end(ctx, claim, standing, unknown)
}

// end closes an abandoned run, saying how much of the world it could not
// return to how it found it.
func (w *Worker) end(ctx context.Context, claim domain.Claim, standing, unknown int) error {
	payload, err := json.Marshal(abandonedFailure(standing, unknown))
	if err != nil {
		return fmt.Errorf("encode ending of %s: %w", claim.RunID, err)
	}
	// Fenced on the lease alone: ending the run is the successor's call once
	// the lease is no longer this worker's.
	if _, err := w.deps.Ledger.AppendIfLeaseAndHead(ctx, claim.Epoch, domain.StepRef{}, domain.Step{
		RunID: claim.RunID, Kind: domain.StepFailed, Scope: claim.Scope,
		AgentID: claim.AgentID, VersionID: claim.VersionID,
		OnBehalfOf: claim.OnBehalfOf, At: w.deps.Clock.Now(), Payload: payload,
	}); err != nil {
		if errors.Is(err, domain.ErrLeaseLost) {
			w.log.Warn("lease lost before the ending; leaving the run to its successor",
				"run", claim.RunID)
			return nil
		}
		return fmt.Errorf("end %s: %w", claim.RunID, err)
	}
	return w.release(ctx, claim, domain.ClaimOutcome{})
}

func abandonedFailure(standing, unknown int) domain.FailedPayload {
	message := fmt.Sprintf(
		"abandoned by a person; %d act(s) could not be undone", standing)
	if unknown > 0 {
		message += fmt.Sprintf(
			"; %d compensation outcome(s) unknown — manual reconciliation required", unknown)
	}
	return domain.FailedPayload{Code: "abandoned", Message: message}
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
