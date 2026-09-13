package connectortools

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

const (
	graviteeReconcileLease = 2 * time.Minute
	graviteeReconcileBatch = 25
)

type GraviteeAttemptReconciler struct {
	runtime  *GraviteeAcceptRuntime
	attempts GraviteeAttemptJournal
	owner    string
	now      func() time.Time
}

func NewGraviteeAttemptReconciler(
	runtime *GraviteeAcceptRuntime, attempts GraviteeAttemptJournal, owner string,
) *GraviteeAttemptReconciler {
	return &GraviteeAttemptReconciler{
		runtime: runtime, attempts: attempts, owner: strings.TrimSpace(owner), now: time.Now,
	}
}

func (r *GraviteeAttemptReconciler) Sweep(ctx context.Context) (int, error) {
	if r == nil || r.runtime == nil || r.attempts == nil || r.owner == "" {
		return 0, errors.New("connector: incomplete Gravitee reconciler")
	}
	attempts, err := r.attempts.ClaimDue(ctx, r.owner, r.now().UTC(),
		graviteeReconcileLease, graviteeReconcileBatch)
	if err != nil {
		return 0, err
	}
	var first error
	for _, attempt := range attempts {
		if _, err := r.reconcile(ctx, attempt); err != nil && first == nil {
			first = err
		}
	}
	return len(attempts), first
}

func (r *GraviteeAttemptReconciler) reconcile(
	ctx context.Context, attempt GraviteeAttempt,
) (engine.ToolResult, error) {
	if attempt.Status == GraviteeAttemptConfirmed || attempt.Status == GraviteeAttemptTerminal {
		return r.runtime.finishKnown(ctx, attempt, true)
	}
	abandoned, err := r.runtime.audit.WasAbandoned(ctx, attempt)
	if err != nil {
		return engine.ToolResult{}, err
	}
	if abandoned {
		return r.runtime.finishAbandoned(ctx, attempt, r.owner)
	}
	if attempt.Status == GraviteeAttemptManual {
		return r.runtime.reconcileManual(ctx, attempt, r.owner)
	}
	snapshot, err := r.runtime.snapshotForAttempt(ctx, attempt)
	if err != nil {
		result, rescheduleErr := r.runtime.reschedule(ctx, attempt, r.owner, true)
		return result, rescheduleErr
	}
	if attempt.Status == GraviteeAttemptPrepared {
		result, err := r.runtime.executePrepared(ctx, attempt, snapshot, r.owner, true)
		return result, err
	}
	if attempt.Status == GraviteeAttemptPending {
		result, err := r.runtime.reconcilePending(ctx, attempt, snapshot, r.owner)
		return result, err
	}
	return engine.ToolResult{}, nil
}

func (g *GraviteeAcceptRuntime) finishAbandoned(
	ctx context.Context, attempt GraviteeAttempt, claimedBy string,
) (engine.ToolResult, error) {
	snapshot, err := g.snapshotForAttempt(ctx, attempt)
	if err != nil {
		return engine.ToolResult{}, err
	}
	return g.finalize(ctx, attempt, claimedBy, snapshot,
		GraviteeAttemptTerminal, CodeConnectorAbandoned, "", true)
}

func (g *GraviteeAcceptRuntime) snapshotForAttempt(
	ctx context.Context, attempt GraviteeAttempt,
) (GraviteeSnapshot, error) {
	raw, err := g.content.Get(ctx, attempt.Execution.Snapshot.Ref)
	if err != nil {
		return GraviteeSnapshot{}, ErrGraviteeEvidence
	}
	evidence := domain.ApprovalEvidence{
		Kind: ApprovalEvidenceGraviteeSubscription, Ticket: attempt.Execution.Ref,
		Ref: attempt.Execution.Snapshot.Ref, Digest: attempt.Execution.Snapshot.Digest,
	}
	snapshot, err := DecodeGraviteeApprovalEvidence(raw, evidence)
	if err != nil || snapshot.SubscriptionID != attempt.SubscriptionID {
		return GraviteeSnapshot{}, ErrGraviteeEvidence
	}
	return snapshot, nil
}

func (g *GraviteeAcceptRuntime) reconcilePending(
	ctx context.Context, attempt GraviteeAttempt, snapshot GraviteeSnapshot, claimedBy string,
) (engine.ToolResult, error) {
	if !g.now().UTC().Before(attempt.DeadlineAt) {
		return g.markManual(ctx, attempt, claimedBy, CodeConnectorNeedsAttention, true)
	}
	access, err := g.access.Resolve(ctx, attempt.Instance, attempt.Scope)
	if err != nil || access.ContractDigest != attempt.ContractDigest {
		return g.reschedule(ctx, attempt, claimedBy, true)
	}
	observed, err := g.remote.Observe(ctx, access.Config, access.credential,
		attempt.SubscriptionID)
	if err != nil {
		return g.reschedule(ctx, attempt, claimedBy, true)
	}
	switch observed.Status {
	case "PENDING":
		if !sameApprovedSnapshot(snapshot, observed) {
			return g.markManual(ctx, attempt, claimedBy, CodeConnectorSnapshotChanged, true)
		}
		return g.reschedule(ctx, attempt, claimedBy, true)
	case "ACCEPTED":
		if !sameAcceptedSnapshot(snapshot, observed) {
			return g.markManual(ctx, attempt, claimedBy, CodeConnectorSnapshotChanged, true)
		}
		return g.finalize(ctx, attempt, claimedBy, snapshot,
			GraviteeAttemptConfirmed, "accepted", observed.UpdatedAt, true)
	case "REJECTED", "CLOSED":
		if !sameSubscriptionIdentity(snapshot, observed) {
			return g.markManual(ctx, attempt, claimedBy, CodeConnectorSnapshotChanged, true)
		}
		return g.finalize(ctx, attempt, claimedBy, snapshot,
			GraviteeAttemptTerminal, CodeConnectorUpstreamFailed, "", true)
	default:
		return g.reschedule(ctx, attempt, claimedBy, true)
	}
}

func (g *GraviteeAcceptRuntime) reconcileManual(
	ctx context.Context, attempt GraviteeAttempt, claimedBy string,
) (engine.ToolResult, error) {
	snapshot, err := g.snapshotForAttempt(ctx, attempt)
	if err != nil {
		return g.rescheduleManual(ctx, attempt, claimedBy)
	}
	access, err := g.access.Resolve(ctx, attempt.Instance, attempt.Scope)
	if err != nil || access.ContractDigest != attempt.ContractDigest {
		return g.rescheduleManual(ctx, attempt, claimedBy)
	}
	observed, err := g.remote.Observe(ctx, access.Config, access.credential,
		attempt.SubscriptionID)
	if err != nil {
		return g.rescheduleManual(ctx, attempt, claimedBy)
	}
	switch observed.Status {
	case "ACCEPTED":
		if sameAcceptedSnapshot(snapshot, observed) {
			return g.finalize(ctx, attempt, claimedBy, snapshot,
				GraviteeAttemptConfirmed, "accepted", observed.UpdatedAt, true)
		}
	case "REJECTED", "CLOSED":
		if sameSubscriptionIdentity(snapshot, observed) {
			return g.finalize(ctx, attempt, claimedBy, snapshot,
				GraviteeAttemptTerminal, CodeConnectorUpstreamFailed, "", true)
		}
	}
	return g.rescheduleManual(ctx, attempt, claimedBy)
}

func (g *GraviteeAcceptRuntime) rescheduleManual(
	ctx context.Context, attempt GraviteeAttempt, claimedBy string,
) (engine.ToolResult, error) {
	now := g.now().UTC()
	_, err := g.attempts.Resolve(ctx, GraviteeAttemptResolution{
		IdemKey: attempt.IdemKey, ClaimedBy: claimedBy, Status: GraviteeAttemptManual,
		Result: attempt.Result, OutcomeCode: attempt.OutcomeCode,
		NextCheckAt: now.Add(graviteeManualCheck), At: now,
	})
	return failed(attempt.OutcomeCode), err
}

func sameSubscriptionIdentity(snapshot GraviteeSnapshot, observed GraviteeObservation) bool {
	return observed.SubscriptionID == snapshot.SubscriptionID &&
		observed.Application == snapshot.Application && observed.API == snapshot.API &&
		observed.Plan == snapshot.Plan && observed.PlanSecurity == snapshot.PlanSecurity
}
