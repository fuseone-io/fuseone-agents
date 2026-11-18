package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// simulatedAwaitingApproval is a simulated run of a battery, held where the
// Gate asked for a person.
func simulatedAwaitingApproval(t *testing.T) *ledger.Memory {
	t.Helper()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	at := time.Date(2026, 11, 18, 14, 0, 0, 0, time.UTC)
	for _, step := range []domain.Step{
		{RunID: "run-sim", Kind: domain.StepRunStarted, Scope: scope, AgentID: "triage", VersionID: "v1", At: at,
			Payload: mustPayload(t, domain.RunStartedPayload{Trigger: "simulation", Simulated: true, Simulation: "sim-1", Case: "estorno"})},
		{RunID: "run-sim", Kind: domain.StepApprovalRequested, Scope: scope, AgentID: "triage", VersionID: "v1", At: at,
			Payload: mustPayload(t, domain.ApprovalRequestedPayload{Tool: "crm.reply", Rule: "taint"})},
	} {
		if _, err := store.Append(context.Background(), step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}
	return store
}

/*
A simulated run is hidden from the inbox, but a direct call by somebody who
holds approval:act would record a real approval_decided on it: a decision
nobody made about an action that never happens, in the trail every audit
reads (NT-018 phase 0). Refused, and nothing is written, whichever way the
decision goes.
*/
func TestDecideApproval_aSimulatedRunHeldForAPerson_isRefusedAndRecordsNothing(t *testing.T) {
	t.Parallel()
	for _, approved := range []bool{true, false} {
		store := simulatedAwaitingApproval(t)
		resp, err := NewServer(store, "test").DecideApproval(inArea("cx", domain.RoleApprover),
			openapi.DecideApprovalRequestObject{RunId: "run-sim",
				Body: &openapi.DecideApprovalJSONRequestBody{Approved: approved, AtSeq: 2}})
		if err != nil {
			t.Fatalf("DecideApproval: %v", err)
		}
		refused, ok := resp.(openapi.DecideApproval409ApplicationProblemPlusJSONResponse)
		if !ok || refused.Type == nil || *refused.Type != string(CodeSimulatedRun) {
			t.Fatalf("approved=%v: answer = %#v, want 409 %s", approved, resp, CodeSimulatedRun)
		}
		steps, _ := store.Read(context.Background(), "run-sim", domain.FirstSeq)
		if len(steps) != 2 {
			t.Errorf("approved=%v: %d steps, want the two seeded and no decision", approved, len(steps))
		}
	}
}
