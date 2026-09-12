package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ticket"
)

const (
	graviteeFirstCheck  = 35 * time.Second
	graviteeDeadline    = 10 * time.Minute
	graviteeClaimLease  = 2 * time.Minute
	graviteeManualCheck = 15 * time.Minute
)

type GraviteeAcceptanceRemote interface {
	Inspect(context.Context, GraviteeConfig, SecretValue, string) (GraviteeObservation, error)
	Observe(context.Context, GraviteeConfig, SecretValue, string) (GraviteeObservation, error)
	Accept(context.Context, GraviteeConfig, SecretValue, GraviteeSnapshot) (GraviteeObservation, error)
}

type GraviteeAcceptRuntime struct {
	access   GraviteeAccesses
	remote   GraviteeAcceptanceRemote
	content  engine.ContentStore
	tickets  ticket.Store
	attempts GraviteeAttemptJournal
	audit    GraviteeReconciliations
	now      func() time.Time
}

func NewGraviteeAcceptRuntime(
	access GraviteeAccesses, remote GraviteeAcceptanceRemote,
	content engine.ContentStore, tickets ticket.Store, attempts GraviteeAttemptJournal,
	audit GraviteeReconciliations,
) *GraviteeAcceptRuntime {
	return &GraviteeAcceptRuntime{
		access: access, remote: remote, content: content,
		tickets: tickets, attempts: attempts, audit: audit, now: time.Now,
	}
}

func (g *GraviteeAcceptRuntime) Accept(
	ctx context.Context, instance string, call engine.Call, input GraviteeInspectInput,
) (engine.ToolResult, error) {
	if err := g.validateCall(call); err != nil {
		return failed(CodeConnectorBadArguments), nil
	}
	snapshot, err := g.approvedSnapshot(ctx, call, input)
	if err != nil {
		return failed(CodeConnectorSnapshotChanged), nil
	}
	if stored, err := g.attempts.Get(ctx, call.IdemKey); err == nil {
		if !sameAttemptCall(stored, instance, call, snapshot) {
			return engine.ToolResult{}, ticket.ErrAttemptConflict
		}
		return g.finishKnown(ctx, stored, false)
	} else if !errors.Is(err, ErrGraviteeAttemptNotFound) {
		return engine.ToolResult{}, err
	}
	attempt, created, err := g.claimTicket(ctx, instance, call, snapshot)
	if err != nil {
		return failed(CodeConnectorSnapshotChanged), nil
	}
	if !created {
		stored, err := g.attempts.Get(ctx, call.IdemKey)
		if err != nil {
			return engine.ToolResult{}, err
		}
		return g.finishKnown(ctx, stored, false)
	}
	return g.executePrepared(ctx, attempt, snapshot, attempt.ClaimedBy, false)
}

func sameAttemptCall(
	attempt GraviteeAttempt, instance string, call engine.Call, snapshot GraviteeSnapshot,
) bool {
	return attempt.IdemKey == call.IdemKey && attempt.Execution.Ref == call.Ticket.Ref &&
		attempt.Execution.RunID == call.RunID &&
		attempt.Execution.ApprovalAtSeq == call.ApprovalAtSeq &&
		attempt.Execution.Snapshot.Ref == call.ApprovalEvidence.Ref &&
		attempt.Execution.Snapshot.Digest == call.ApprovalEvidence.Digest &&
		attempt.CallSeq == call.Seq && attempt.Instance == instance &&
		attempt.Scope == call.Scope && attempt.ContractDigest == call.ContractDigest &&
		attempt.SubscriptionID == snapshot.SubscriptionID && attempt.DecidedBy == call.DecidedBy
}

func (g *GraviteeAcceptRuntime) validateCall(call engine.Call) error {
	if g == nil || g.access == nil || g.remote == nil || g.content == nil ||
		g.tickets == nil || g.attempts == nil || g.audit == nil || !call.Ticket.Valid() ||
		call.ApprovalAtSeq <= 0 || call.ApprovalEvidence.Ticket != call.Ticket.Ref ||
		call.ApprovalEvidence.Kind != ApprovalEvidenceGraviteeSubscription ||
		!call.ApprovalEvidence.Valid() || call.DecidedBy == "" || call.IdemKey == "" ||
		call.Seq <= 0 || call.At.IsZero() {
		return ErrGraviteeEvidence
	}
	return nil
}

func (g *GraviteeAcceptRuntime) approvedSnapshot(
	ctx context.Context, call engine.Call, input GraviteeInspectInput,
) (GraviteeSnapshot, error) {
	raw, err := g.content.Get(ctx, call.ApprovalEvidence.Ref)
	if err != nil {
		return GraviteeSnapshot{}, ErrGraviteeEvidence
	}
	snapshot, err := DecodeGraviteeApprovalEvidence(raw, call.ApprovalEvidence)
	if err != nil || snapshot.SubscriptionID != input.SubscriptionID ||
		!expirationInputMatches(input.ExpiresAt, snapshot.RequestedExpiration) {
		return GraviteeSnapshot{}, ErrGraviteeEvidence
	}
	return snapshot, nil
}

func expirationInputMatches(raw string, approved *string) bool {
	if raw == "" {
		return approved == nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || approved == nil {
		return false
	}
	return parsed.UTC().Format(time.RFC3339Nano) == *approved
}

func (g *GraviteeAcceptRuntime) claimTicket(
	ctx context.Context, instance string, call engine.Call, snapshot GraviteeSnapshot,
) (GraviteeAttempt, bool, error) {
	approvedRef := ticket.ContentRef{
		Ref: call.ApprovalEvidence.Ref, Digest: call.ApprovalEvidence.Digest,
	}
	if _, _, err := g.tickets.AwaitApproval(ctx, ticket.ApprovalInput{
		Ref: call.Ticket.Ref, RunID: call.RunID, AtSeq: call.ApprovalAtSeq,
		Snapshot: approvedRef, At: call.At,
	}); err != nil {
		return GraviteeAttempt{}, false, err
	}
	started := call.At.UTC()
	owner := "invoke:" + string(call.RunID) + ":" + strconv.FormatInt(call.Seq, 10)
	_, external, created, err := g.tickets.ClaimExecutionWithAttempt(ctx, ticket.ClaimAttemptInput{
		Claim: ticket.ClaimInput{
			Ref: call.Ticket.Ref, RunID: call.RunID,
			ApprovalAtSeq: call.ApprovalAtSeq, At: started,
		},
		Attempt: ticket.ExternalAttemptInput{
			IdemKey: call.IdemKey, Kind: graviteeAttemptKind, CallSeq: call.Seq,
			Instance: instance, Scope: call.Scope, ContractDigest: call.ContractDigest,
			TargetID: snapshot.SubscriptionID, DecidedBy: call.DecidedBy,
			NextCheckAt: started, DeadlineAt: started.Add(graviteeDeadline),
			ClaimedBy: owner, ClaimedUntil: started.Add(graviteeClaimLease),
		},
	})
	if err != nil {
		return GraviteeAttempt{}, false, err
	}
	attempt, err := graviteeAttemptFromExternal(external)
	return attempt, created, err
}

func (g *GraviteeAcceptRuntime) executePrepared(
	ctx context.Context, attempt GraviteeAttempt, snapshot GraviteeSnapshot,
	claimedBy string, recovered bool,
) (engine.ToolResult, error) {
	if !g.now().UTC().Before(attempt.DeadlineAt) {
		return g.markManual(ctx, attempt, claimedBy, CodeConnectorNeedsAttention, recovered)
	}
	access, err := g.access.Resolve(ctx, attempt.Instance, attempt.Scope)
	if err != nil {
		return g.reschedule(ctx, attempt, claimedBy, recovered)
	}
	if access.ContractDigest != attempt.ContractDigest {
		return g.finalize(ctx, attempt, claimedBy, snapshot,
			GraviteeAttemptTerminal, CodeConnectorContractChanged, "", recovered)
	}
	observed, err := g.remote.Inspect(ctx, access.Config, access.credential, snapshot.SubscriptionID)
	if err != nil {
		if errors.Is(err, ErrGraviteeState) || errors.Is(err, ErrGraviteeScope) ||
			errors.Is(err, ErrGraviteeRequester) {
			return g.finalize(ctx, attempt, claimedBy, snapshot,
				GraviteeAttemptTerminal, CodeConnectorSnapshotChanged, "", recovered)
		}
		return g.reschedule(ctx, attempt, claimedBy, recovered)
	}
	if !sameApprovedSnapshot(snapshot, observed) {
		return g.finalize(ctx, attempt, claimedBy, snapshot,
			GraviteeAttemptTerminal, CodeConnectorSnapshotChanged, "", recovered)
	}
	now := g.now().UTC()
	armed, err := g.attempts.Arm(ctx, attempt.IdemKey, claimedBy,
		now.Add(graviteeFirstCheck), now)
	if err != nil {
		return engine.ToolResult{}, err
	}
	accepted, err := g.remote.Accept(ctx, access.Config, access.credential, snapshot)
	if err != nil {
		if ambiguousGraviteeResult(err) {
			return g.unknown(ctx, armed, "")
		}
		return g.finalize(ctx, armed, "", snapshot,
			GraviteeAttemptTerminal, CodeConnectorUpstreamFailed, "", recovered)
	}
	if !sameAcceptedSnapshot(snapshot, accepted) {
		return g.unknown(ctx, armed, "")
	}
	return g.finalize(ctx, armed, "", snapshot,
		GraviteeAttemptConfirmed, "accepted", accepted.UpdatedAt, recovered)
}

func sameApprovedSnapshot(snapshot GraviteeSnapshot, observed GraviteeObservation) bool {
	want := snapshotOf(snapshot.TicketRef, observed, snapshot.RequestedExpiration)
	return sameSnapshot(want, snapshot) && observed.EndingAt == nil
}

func sameSnapshot(a, b GraviteeSnapshot) bool {
	return a.TicketRef == b.TicketRef && a.SubscriptionID == b.SubscriptionID &&
		a.Status == b.Status && a.Application == b.Application && a.API == b.API &&
		a.Plan == b.Plan && a.PlanSecurity == b.PlanSecurity &&
		sameRequestedExpiration(a.RequestedExpiration, b.RequestedExpiration) &&
		a.RemoteCreatedAt == b.RemoteCreatedAt && a.RemoteUpdatedAt == b.RemoteUpdatedAt
}

func sameAcceptedSnapshot(snapshot GraviteeSnapshot, observed GraviteeObservation) bool {
	return observed.Status == "ACCEPTED" && observed.SubscriptionID == snapshot.SubscriptionID &&
		observed.Application == snapshot.Application && observed.API == snapshot.API &&
		observed.Plan == snapshot.Plan && observed.PlanSecurity == snapshot.PlanSecurity &&
		sameRequestedExpiration(observed.EndingAt, snapshot.RequestedExpiration)
}

func (g *GraviteeAcceptRuntime) reschedule(
	ctx context.Context, attempt GraviteeAttempt, claimedBy string, recovered bool,
) (engine.ToolResult, error) {
	now := g.now().UTC()
	if !now.Before(attempt.DeadlineAt) {
		return g.markManual(ctx, attempt, claimedBy, CodeConnectorNeedsAttention, recovered)
	}
	next := now.Add(graviteeRecheckDelay(attempt.Checks))
	if next.After(attempt.DeadlineAt) {
		next = attempt.DeadlineAt
	}
	_, err := g.attempts.Resolve(ctx, GraviteeAttemptResolution{
		IdemKey: attempt.IdemKey, ClaimedBy: claimedBy, Status: attempt.Status,
		NextCheckAt: next, At: now,
	})
	return failed(CodeConnectorOutcomeUnknown), err
}

func (g *GraviteeAcceptRuntime) markManual(
	ctx context.Context, attempt GraviteeAttempt, claimedBy, code string, recovered bool,
) (engine.ToolResult, error) {
	now := g.now().UTC()
	raw, err := json.Marshal(GraviteeAcceptanceResult{
		Operation: "gravitee.accept_subscription", Status: CodeConnectorNeedsAttention,
		SubscriptionID: attempt.SubscriptionID, DecidedBy: attempt.DecidedBy,
		Recovered: recovered,
	})
	if err != nil {
		return engine.ToolResult{}, err
	}
	ref, err := g.content.Put(ctx, attempt.Execution.RunID, attempt.CallSeq, raw)
	if err != nil {
		return engine.ToolResult{}, err
	}
	content := ticket.ContentRef{Ref: ref, Digest: engine.ResultDigest(raw)}
	resolved, err := g.attempts.Resolve(ctx, GraviteeAttemptResolution{
		IdemKey: attempt.IdemKey, ClaimedBy: claimedBy, Status: GraviteeAttemptManual,
		Result: content, OutcomeCode: code, NextCheckAt: now.Add(graviteeManualCheck), At: now,
	})
	if err != nil {
		return engine.ToolResult{}, err
	}
	return g.finishManual(ctx, resolved, recovered)
}

func (g *GraviteeAcceptRuntime) unknown(
	_ context.Context, _ GraviteeAttempt, _ string,
) (engine.ToolResult, error) {
	// Arm already made the uncertainty durable and scheduled the first GET.
	// Moving that deadline here would collapse the deliberate post-write quiet
	// period and amplify load while Gravitee is still converging.
	return failed(CodeConnectorOutcomeUnknown), nil
}

func graviteeRecheckDelay(checks int) time.Duration {
	delay := 5 * time.Second
	for i := 0; i < checks && delay < time.Minute; i++ {
		delay *= 2
	}
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}
