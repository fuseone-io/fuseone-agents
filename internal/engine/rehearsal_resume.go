package engine

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
)

/*
awaitWorld records that a rehearsal's call waits for its author (NT-018 phase 4). It is
also how a rehearsal resumes a call a crashed worker left behind: a rehearsal's call never
reaches the world, so there is no unknown outcome to record, only an answer still owed.
*/
func (r *Runner) awaitWorld(ctx context.Context, state State, start Start) (Status, error) {
	state, err := r.append(ctx, state, start, domain.Step{
		Kind:    domain.StepWorldAwaited,
		Labels:  state.Labels.Clone(),
		Payload: mustJSON(domain.WorldAwaitedPayload{Tool: state.PendingTool, CallSeq: state.pendingCallSeq}),
	})
	return status(state), err
}

/*
returnSupplied records the author's answer as the call's result, then releases what the
call reserved, as every call does on every path. The answer is untrusted: as far as the
agent's next decision goes it is external content, and calling it trusted would let a
rehearsal take a path production would refuse. A worker that crashed between the answer
and this result lands here again on the next turn, and records it then.
*/
func (r *Runner) returnSupplied(ctx context.Context, state State, start Start) (Status, error) {
	answer, outstanding := *state.supplied, state.Reserved
	labels := state.Labels.Union(domain.NewLabels(domain.LabelUntrusted))
	state, err := r.append(ctx, state, start, domain.Step{
		Kind:   domain.StepToolReturned,
		Labels: labels,
		Payload: mustJSON(domain.ToolReturnedPayload{
			Tool: state.PendingTool, ForSeq: state.pendingCallSeq,
			ResultRef: answer.Ref, ResultDigest: answer.Digest, ResultBytes: answer.Bytes,
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
