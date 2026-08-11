package engine

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// Resuming a run: rebuilding its state from the ledger and closing out
// whatever the previous worker left half-done.

// load folds the run's ledger, opening the run if it has no steps yet.
// recoverOrphanedCall closes out a tool call whose result never came back,
// which is what a worker sees after it dies between the call and the response.
//
// The outcome is genuinely unknown: the effect may or may not have landed. The
// loop records that honestly rather than guessing, releases the reservation so
// the budget does not leak, and lets the next turn re-plan. Safety against a
// repeat comes from the idempotency key already recorded with the call, not
// from assuming the call failed.
func (r *Runner) recoverOrphanedCall(ctx context.Context, state State, start Start) (Status, error) {
	outstanding := state.Reserved

	state, err := r.append(ctx, state, start, domain.Step{
		Kind: domain.StepToolReturned,
		Payload: mustJSON(domain.ToolReturnedPayload{
			Tool:      state.PendingTool,
			Failed:    true,
			ErrorCode: "unknown_outcome_after_restart",
		}),
	})
	if err != nil {
		return Status{}, err
	}

	state, err = r.append(ctx, state, start, domain.Step{
		Kind: domain.StepBudgetReconciled,
		Payload: mustJSON(domain.BudgetReconciledPayload{
			ReleasedMicros: outstanding.Micros, ReleasedTokens: outstanding.Tokens,
		}),
	})
	return status(state), err
}

func (r *Runner) load(ctx context.Context, start Start) (State, error) {
	steps, err := r.deps.Ledger.Read(ctx, start.RunID, domain.FirstSeq)
	if err != nil && !isNotFound(err) {
		return State{}, fmt.Errorf("engine: read ledger: %w", err)
	}

	state, err := Fold(steps)
	if err != nil {
		return State{}, fmt.Errorf("engine: fold: %w", err)
	}
	if state.Phase != PhaseUnstarted {
		return state, nil
	}

	return r.append(ctx, state, start, domain.Step{
		Kind:    domain.StepRunStarted,
		Payload: mustJSON(domain.RunStartedPayload{Trigger: start.Trigger}),
	})
}
