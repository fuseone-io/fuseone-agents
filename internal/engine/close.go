package engine

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
Closer seals a run's closing answer in place of the engine's own outcome (NT-016).

A judge's answer is a verdict, and what the engine would store of it — the answer as
written, a headline, artifacts — is the judge's words, unsalted and of whatever length the
value made them. A run with a Closer stores what the Closer sealed and nothing else.
*/
type Closer interface {
	Close(p Proposal) Closing
}

// Closing is what a Closer sealed. Outcome and Judgment are stored as they are, nil
// meaning nothing to store; Reason is the closed code the run finishes with.
type Closing struct {
	Outcome  []byte
	Judgment []byte
	Reason   domain.RunFinishedReason
}

// finishClosed is the closing of a run with a Closer, at the sequence about to be written.
func (r *Runner) finishClosed(
	ctx context.Context, state State, start Start, proposal Proposal,
) (Status, error) {
	closing := start.Closer.Close(proposal)
	finishSeq := state.Seq + 1
	outcomeRef, err := r.storeSealed(ctx, start.RunID, finishSeq, closing.Outcome)
	if err != nil {
		return Status{}, err
	}
	judgmentRef, err := r.storeSealed(ctx, start.RunID, finishSeq, closing.Judgment)
	if err != nil {
		return Status{}, err
	}
	state, err = r.append(ctx, state, start, domain.Step{
		Kind:   domain.StepRunFinished,
		Labels: state.Labels.Clone(),
		Payload: mustJSON(domain.RunFinishedPayload{
			OutcomeRef: outcomeRef, OutcomeDigest: sealedDigest(closing.Outcome),
			JudgmentRef: judgmentRef, JudgmentDigest: sealedDigest(closing.Judgment),
			Reason: closing.Reason,
		}),
	})
	return status(state), err
}

// storeSealed stores a sealed object, and nothing when there is none.
func (r *Runner) storeSealed(ctx context.Context, runID domain.RunID, seq int64, data []byte) (string, error) {
	if data == nil {
		return "", nil
	}
	if r.deps.Content == nil {
		return "", fmt.Errorf("engine: no content store to hold the closing of %s", runID)
	}
	ref, err := r.deps.Content.Put(ctx, runID, seq, data)
	if err != nil {
		return "", fmt.Errorf("engine: store closing: %w", err)
	}
	return ref, nil
}

func sealedDigest(data []byte) string {
	if data == nil {
		return ""
	}
	return digest(data)
}
