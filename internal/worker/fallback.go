package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/model"
)

// Fallback is the pair a version declared for when its own keeps failing.
type Fallback struct {
	// From is the declared pair, To the fallback, both as the author
	// reviewed them.
	From, To domain.ModelPair
	// Planner plans with To. It is handed over on every turn and asked only
	// once the run's ledger records the switch.
	Planner engine.Planner
}

/*
switchCodes are the failures a different provider can outlast.

A closed list rather than "anything retryable". Retryable also covers a run
waiting on another run's effect reservation and a network this worker cannot
cross, and a vendor switch fixes neither. Authentication and bad requests are
not retryable at all: they are this installation's mistake, and asking a
second vendor would hide it.
*/
var switchCodes = map[string]bool{
	model.CodeProviderOverloaded:  true,
	model.CodeRateLimited:         true,
	model.CodeProviderUnavailable: true,
}

// switchable is the failure that would move a run onto fb, if any.
func switchable(fb *Fallback, err error) (domain.FailureSummary, bool) {
	if fb == nil {
		return domain.FailureSummary{}, false
	}
	failure, ok := model.FailureSummaryOf(err)
	if !ok || !failure.Retryable || !switchCodes[failure.Code] {
		return domain.FailureSummary{}, false
	}
	return failure, true
}

/*
exhausted is what a run whose attempts ran out does instead of parking, when
its version declared a fallback. It reports whether it took the run.

Read from the ledger, not from this worker: whichever worker exhausted the
attempts, the switch is recorded where every later turn folds it from. A run
already on its fallback parks as it always did — sticky means no switching
back, and nothing left to switch to.

An autonomous run does not switch. It was promoted on evidence gathered
against its declared pair, and nothing has rehearsed it against the other; it
parks under a reason that says so, beside the provider's own failure.
*/
func (w *Worker) exhausted(
	ctx context.Context, claim domain.Claim, start engine.Start, fb *Fallback, advErr error,
) (bool, error) {
	failure, ok := switchable(fb, advErr)
	if !ok {
		return false, nil
	}
	state, err := w.fold(ctx, claim.RunID)
	if err != nil {
		// Parking is what the run would have done without a fallback, and
		// switching on a reading nobody could make is not safer than that.
		w.log.Error("could not read the run before its fallback", "run", claim.RunID, "err", err)
		return false, nil
	}
	if state.Fallback != nil || state.Phase != engine.PhaseRunning {
		return false, nil
	}
	if start.Stage == domain.StageAutonomous {
		w.metrics.AdvanceFailure(failure.Code, true)
		return true, w.park(ctx, claim, advErr, domain.ParkFallbackUnrehearsed)
	}
	return true, w.switchTo(ctx, claim, state, start, fb, failure, advErr)
}

// switchTo records the switch and hands the run back with its attempts reset:
// the fallback gets the same patience the declared pair had.
func (w *Worker) switchTo(
	ctx context.Context, claim domain.Claim, state engine.State, start engine.Start,
	fb *Fallback, failure domain.FailureSummary, advErr error,
) error {
	from := fb.From
	// The pair that failed, which is the step's own model when it named one.
	if stepModel, _ := engine.SpendAt(start, state.Called); stepModel != "" {
		from.Model = stepModel
	}
	payload, err := json.Marshal(domain.ModelFallbackPayload{
		From: from, To: fb.To, Failure: failure, Attempts: claim.Attempts + 1,
	})
	if err != nil {
		return fmt.Errorf("encode the fallback of %s: %w", claim.RunID, err)
	}
	// Fenced like a parking: a worker that lost the run must not switch it
	// under its successor, and a switch decided before somebody acted on the
	// run must not be sealed on top of what they did.
	_, err = w.deps.Ledger.AppendIfLeaseAndHead(ctx, claim.Epoch, state.Head, domain.Step{
		RunID: claim.RunID, Kind: domain.StepModelFallback, Scope: claim.Scope,
		AgentID: claim.AgentID, VersionID: claim.VersionID, OnBehalfOf: claim.OnBehalfOf,
		At: w.clock.Now(), Payload: payload,
	})
	switch {
	case errors.Is(err, domain.ErrLeaseLost):
		w.log.Warn("lease lost before the fallback; leaving the run to its successor", "run", claim.RunID)
		return nil
	case errors.Is(err, domain.ErrHeadMoved):
		w.log.Warn("the run moved before the fallback; standing down", "run", claim.RunID)
		return w.release(ctx, claim, domain.ClaimOutcome{})
	case err != nil:
		// Not recorded means not switched: the run parks as it would have.
		w.log.Error("could not record the fallback", "run", claim.RunID, "err", err)
		return w.park(ctx, claim, advErr, parkedReasonFor(advErr))
	}
	w.metrics.AdvanceFailure(failure.Code, false)
	w.log.Warn("switched to the declared fallback", "run", claim.RunID,
		"from", from.Provider, "to", fb.To.Provider, "code", failure.Code)
	return w.release(ctx, claim, domain.ClaimOutcome{})
}

func (w *Worker) fold(ctx context.Context, run domain.RunID) (engine.State, error) {
	steps, err := w.deps.Ledger.Read(ctx, run, domain.FirstSeq)
	if err != nil {
		return engine.State{}, fmt.Errorf("read %s: %w", run, err)
	}
	return engine.Fold(steps)
}

// failed supervises a turn that did not advance: retry later, switch to the
// declared fallback, or park for a person.
func (w *Worker) failed(
	ctx context.Context, claim domain.Claim, start engine.Start, fb *Fallback, advErr error, log *slog.Logger,
) error {
	outcome := w.failure(claim, advErr)
	if outcome.Parked {
		if took, err := w.exhausted(ctx, claim, start, fb, advErr); took {
			return err
		}
	}
	w.metrics.AdvanceFailure(failureReasonFor(advErr), outcome.Parked)
	log.Warn("advance failed",
		"attempt", claim.Attempts+1, "parked", outcome.Parked, "err", advErr)
	if outcome.Parked {
		return w.park(ctx, claim, advErr, parkedReasonFor(advErr))
	}
	return w.release(ctx, claim, outcome)
}
