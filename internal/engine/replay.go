package engine

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
)

/*
Replaying a case's world (NT-018 phase 5). The engine answers each call of a replayed case
from the world the case was recorded with, strictly in order and each entry once; a tool
layer is never asked, not even to get ready. The answer is pure, so a call a crashed worker
left behind is resolved again the same way, and there is no unknown outcome to record.

The first call that leaves the world is written down here, once, as replay_diverged, and
every call after it is answered with nothing: the case is no longer the situation the author
corrected, and the report says so rather than judge it.
*/

// replayFold is how far a replayed case has followed its world.
type replayFold struct {
	// consumed is how many entries were handed out; pendingArgs, the digest of the arguments
	// of the call waiting for its result.
	consumed    int
	pendingArgs string
	diverged    bool
}

// resolveReplay answers the pending call from the world, then releases what it reserved.
func (r *Runner) resolveReplay(ctx context.Context, state State, start Start) (Status, error) {
	answer := domain.ToolReturnedPayload{Tool: state.PendingTool, ForSeq: state.pendingCallSeq}
	labels := state.Labels.Clone()
	if !state.replay.diverged {
		entry, divergence := start.World.Match(state.replay.consumed, state.PendingTool, state.replay.pendingArgs)
		if divergence != domain.DivergenceNone {
			var err error
			if state, err = r.diverge(ctx, state, start, divergence, state.PendingTool); err != nil {
				return Status{}, err
			}
		} else {
			answer.ResultRef, answer.ResultDigest, answer.ResultBytes = entry.AnswerRef, entry.AnswerDigest, entry.AnswerBytes
			answer.Replayed = entry.Position
			labels = labels.Union(domain.NewLabels(domain.LabelUntrusted))
		}
	}
	outstanding := state.Reserved
	state, err := r.append(ctx, state, start, domain.Step{
		Kind: domain.StepToolReturned, Labels: labels, Payload: mustJSON(answer),
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

// closeWorld records, before a replayed run finishes, the entries it never asked for.
func (r *Runner) closeWorld(ctx context.Context, state State, start Start) (State, error) {
	if start.World == nil || state.replay.diverged || state.replay.consumed >= len(start.World.Entries) {
		return state, nil
	}
	return r.diverge(ctx, state, start, domain.DivergenceMissing, start.World.Entries[state.replay.consumed].Tool)
}

// diverge writes the first divergence: where the world stood, how the run left it, and the
// tool it called, or never called.
func (r *Runner) diverge(
	ctx context.Context, state State, start Start, divergence domain.Divergence, tool domain.ToolID,
) (State, error) {
	return r.append(ctx, state, start, domain.Step{
		Kind: domain.StepReplayDiverged,
		Payload: mustJSON(domain.ReplayDivergedPayload{
			Position: state.replay.consumed + 1, Divergence: divergence, Tool: tool,
		}),
	})
}
