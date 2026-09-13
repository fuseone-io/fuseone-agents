package connectortools

import (
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/ticket"
)

func TestGraviteeReconciliation_sealsARecoveredResultOnce(t *testing.T) {
	store := ledger.NewMemory()
	attempt, result := reconciliationCall(t, store)
	unknown := domain.ToolReturnedPayload{
		Tool: toolForAttempt(attempt), Failed: true,
		ErrorCode: CodeConnectorOutcomeUnknown,
	}
	appendReconciliationStep(t, store, attempt, domain.StepToolReturned, unknown)
	appendReconciliationStep(t, store, attempt, domain.StepApprovalRequested,
		domain.ApprovalRequestedPayload{Tool: "ops.follow_up", Rule: "review"})

	reconciliations := NewGraviteeReconciliationLedger(store)
	for range 2 {
		if err := reconciliations.Seal(t.Context(), attempt, result, graviteeNow); err != nil {
			t.Fatalf("Seal: %v", err)
		}
	}

	steps, err := store.Read(t.Context(), attempt.Execution.RunID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := countReconciliations(steps); got != 1 {
		t.Fatalf("reconciliations = %d, want exactly one", got)
	}
	last := steps[len(steps)-1]
	var payload domain.EffectReconciledPayload
	if err := json.Unmarshal(last.Payload, &payload); err != nil {
		t.Fatalf("decode reconciliation: %v", err)
	}
	if last.Kind != domain.StepEffectReconciled || payload.ForSeq != attempt.CallSeq ||
		payload.Tool != toolForAttempt(attempt) || payload.ResultRef != result.ResultRef ||
		payload.ResultDigest != result.ResultDigest {
		t.Fatalf("reconciliation = (%+v, %+v)", last, payload)
	}
	state, err := engine.Fold(steps)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	if state.Phase != engine.PhaseAwaitingApproval || state.PendingApproval == nil {
		t.Fatalf("state after audit step = %+v, want the later approval untouched", state)
	}
}

func TestGraviteeReconciliation_aNormalReturnNeedsNoSecondAuditStep(t *testing.T) {
	store := ledger.NewMemory()
	attempt, result := reconciliationCall(t, store)
	returned := domain.ToolReturnedPayload{
		Tool: toolForAttempt(attempt), ResultRef: result.ResultRef,
		ResultDigest: result.ResultDigest, ResultBytes: result.ResultBytes,
	}
	appendReconciliationStep(t, store, attempt, domain.StepToolReturned, returned)

	if err := NewGraviteeReconciliationLedger(store).
		Seal(t.Context(), attempt, result, graviteeNow); err != nil {
		t.Fatalf("Seal: %v", err)
	}
	steps, _ := store.Read(t.Context(), attempt.Execution.RunID, domain.FirstSeq)
	if got := countReconciliations(steps); got != 0 {
		t.Fatalf("reconciliations = %d, want the normal return to be authoritative", got)
	}
}

func TestGraviteeReconciliation_readsAbandonmentFromTheRunRecord(t *testing.T) {
	store := ledger.NewMemory()
	attempt, _ := reconciliationCall(t, store)
	appendReconciliationStep(t, store, attempt, domain.StepAbandoned,
		domain.AbandonedPayload{By: "usr_operator", Reason: "checked in Gravitee"})

	abandoned, err := NewGraviteeReconciliationLedger(store).
		WasAbandoned(t.Context(), attempt)
	if err != nil || !abandoned {
		t.Fatalf("WasAbandoned = (%t, %v), want the recorded decision", abandoned, err)
	}
}

func reconciliationCall(
	t *testing.T, store *ledger.Memory,
) (GraviteeAttempt, engine.ToolResult) {
	t.Helper()
	runID := domain.RunID("run-reconcile")
	appendReconciliationStep(t, store, GraviteeAttempt{Execution: ticket.Execution{RunID: runID}},
		domain.StepRunStarted, domain.RunStartedPayload{})
	attempt := GraviteeAttempt{
		IdemKey: "gravitee-accept-once", Instance: "apim", CallSeq: 2,
		Execution: ticket.Execution{
			Ref: domain.TicketRef{Key: "ticket-1", Revision: 1}, RunID: runID,
			ApprovalAtSeq: 1, Snapshot: ticket.ContentRef{Ref: "snapshot", Digest: "sha256:snapshot"},
		},
	}
	appendReconciliationStep(t, store, attempt, domain.StepToolCalled,
		domain.ToolCalledPayload{Tool: toolForAttempt(attempt), Effect: domain.EffectWrite})
	return attempt, engine.ToolResult{
		ResultRef: "result-ref", ResultDigest: "sha256:result", ResultBytes: 173,
		Labels: domain.NewLabels(domain.LabelUntrusted),
	}
}

func appendReconciliationStep(
	t *testing.T, store *ledger.Memory, attempt GraviteeAttempt,
	kind domain.StepKind, payload any,
) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	step := domain.Step{
		RunID: attempt.Execution.RunID, Kind: kind,
		Scope:   domain.Scope{Company: "acme", Area: "platform"},
		AgentID: "gateway-support", VersionID: "v1", OnBehalfOf: "usr_requester",
		Payload: raw, At: graviteeNow,
	}
	if kind == domain.StepToolCalled {
		step.IdemKey = attempt.IdemKey
	}
	if _, err := store.Append(t.Context(), step); err != nil {
		t.Fatalf("Append(%s): %v", kind, err)
	}
}

func countReconciliations(steps []domain.Step) int {
	count := 0
	for _, step := range steps {
		if step.Kind == domain.StepEffectReconciled {
			count++
		}
	}
	return count
}
