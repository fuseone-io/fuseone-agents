package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// CodeCandidateUnreadable is the failure a run opened on a candidate ends with
// when its seal cannot be honoured (NT-018 phase 2).
const CodeCandidateUnreadable = "fuseone:candidate-unreadable"

// Candidates resolves a run pinned to a seal, declared here by the consumer.
type Candidates interface {
	ResolveCandidate(ctx context.Context, claim domain.Claim, started domain.RunStartedPayload) (Resolution, error)
}

// WithCandidates wires how the simulation pool resolves a run opened on a
// candidate. The real pool never gets it: a candidate never opens a real run.
func (w *Worker) WithCandidates(c Candidates) *Worker {
	w.candidates = c
	return w
}

// resolveCandidate is what a run pinned to a seal plans with. The opener accepts
// a seal only on a simulated run of an experiment or a rehearsal; this is the
// second door, and a pool that cannot read seals keeps it shut.
func (w *Worker) resolveCandidate(
	ctx context.Context, claim domain.Claim, started domain.RunStartedPayload,
) (Resolution, error) {
	if !started.Simulated || (started.Experiment == "" && !started.Rehearsal) {
		return Resolution{}, fmt.Errorf(
			"%s: a seal is honoured only on an experiment's or a rehearsal's simulated run: %w",
			claim.RunID, domain.ErrCandidateUnreadable)
	}
	if w.candidates == nil {
		return Resolution{}, fmt.Errorf("%s: this pool does not read seals: %w",
			claim.RunID, domain.ErrCandidateUnreadable)
	}
	return w.candidates.ResolveCandidate(ctx, claim, started)
}

// unresolved ends a turn whose run could not be resolved. A candidate that
// cannot be honoured will not become readable by retrying, and parking it would
// invite a resume onto the same refusal: it fails. Anything else parks, so
// someone sees it.
func (w *Worker) unresolved(ctx context.Context, claim domain.Claim, reason error) error {
	if !errors.Is(reason, domain.ErrCandidateUnreadable) {
		return w.park(ctx, claim, reason, "spec_unresolved")
	}
	w.log.Warn("candidate unreadable; failing the run", "run", claim.RunID, "err", reason)
	return w.failRun(ctx, claim, domain.FailedPayload{Code: CodeCandidateUnreadable})
}

// failRun ends a run that has not planned, fenced on the lease and the head it
// was decided against, as parking is.
func (w *Worker) failRun(ctx context.Context, claim domain.Claim, failed domain.FailedPayload) error {
	head, err := w.deps.Ledger.Head(ctx, claim.RunID)
	if err != nil {
		return fmt.Errorf("read head before failing %s: %w", claim.RunID, err)
	}
	if head.Kind.Terminal() || head.Kind == domain.StepAbandoned {
		return w.release(ctx, claim, domain.ClaimOutcome{})
	}
	payload, err := json.Marshal(failed)
	if err != nil {
		return fmt.Errorf("encode failure of %s: %w", claim.RunID, err)
	}
	if _, err := w.deps.Ledger.AppendIfLeaseAndHead(ctx, claim.Epoch,
		domain.StepRef{Seq: head.Seq, Kind: head.Kind}, domain.Step{
			RunID: claim.RunID, Kind: domain.StepFailed, Scope: claim.Scope,
			AgentID: claim.AgentID, VersionID: claim.VersionID,
			OnBehalfOf: claim.OnBehalfOf, At: w.deps.Clock.Now(), Payload: payload,
		}); err != nil {
		if errors.Is(err, domain.ErrLeaseLost) || errors.Is(err, domain.ErrHeadMoved) {
			w.log.Warn("the run moved before failing it; leaving it to its owner", "run", claim.RunID)
			return nil
		}
		return fmt.Errorf("fail %s: %w", claim.RunID, err)
	}
	return w.release(ctx, claim, domain.ClaimOutcome{})
}
