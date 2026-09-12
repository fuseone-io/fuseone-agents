package httpapi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

func TestGetApprovalEvidence_returnsTheFixedSafeGraviteeProjection(t *testing.T) {
	t.Parallel()
	server, _, _ := approvalWithGraviteeEvidence(t, false)

	resp, err := server.GetApprovalEvidence(
		inArea("cx", domain.RoleApprover),
		openapi.GetApprovalEvidenceRequestObject{RunId: "run-gravitee", AtSeq: 2},
	)
	if err != nil {
		t.Fatalf("GetApprovalEvidence: %v", err)
	}
	got, ok := resp.(openapi.GetApprovalEvidence200JSONResponse)
	if !ok || got.Kind != openapi.ApprovalEvidenceDetailKindGraviteeSubscription ||
		got.Gravitee == nil {
		t.Fatalf("response = %#v (%T)", resp, resp)
	}
	if got.Gravitee.SubscriptionId != "sub-42" || got.Gravitee.Api.Id != "checkout-api" ||
		got.Gravitee.Application.PrimaryOwnerEmail != "dev@example.com" ||
		got.Gravitee.RequestedExpiration == nil {
		t.Fatalf("Gravitee evidence = %+v", got.Gravitee)
	}
}

func TestDecideApproval_refusesCorruptEvidenceButStillAllowsRejection(t *testing.T) {
	t.Parallel()
	server, store, _ := approvalWithGraviteeEvidence(t, true)
	ctx := inArea("cx", domain.RoleApprover)

	resp, err := server.DecideApproval(ctx, openapi.DecideApprovalRequestObject{
		RunId: "run-gravitee",
		Body:  &openapi.DecideApprovalJSONRequestBody{Approved: true, AtSeq: 2},
	})
	if err != nil {
		t.Fatalf("DecideApproval approve: %v", err)
	}
	if _, conflict := resp.(openapi.DecideApproval409ApplicationProblemPlusJSONResponse); !conflict {
		t.Fatalf("approve response = %T, want 409", resp)
	}
	steps, _ := store.Read(t.Context(), "run-gravitee", domain.FirstSeq)
	if len(steps) != 2 {
		t.Fatalf("steps after refused approval = %d, want 2", len(steps))
	}

	resp, err = server.DecideApproval(ctx, openapi.DecideApprovalRequestObject{
		RunId: "run-gravitee",
		Body:  &openapi.DecideApprovalJSONRequestBody{Approved: false, AtSeq: 2},
	})
	if err != nil {
		t.Fatalf("DecideApproval reject: %v", err)
	}
	if _, ok := resp.(openapi.DecideApproval200JSONResponse); !ok {
		t.Fatalf("reject response = %T, want 200", resp)
	}
}

func TestGetApprovalEvidence_anOrdinaryApprovalDoesNotFetchContent(t *testing.T) {
	t.Parallel()
	store := awaitingApproval(t, time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC))
	resp, err := NewServer(store, "test").GetApprovalEvidence(
		inArea("cx", domain.RoleApprover),
		openapi.GetApprovalEvidenceRequestObject{RunId: "run-1", AtSeq: 2},
	)
	if err != nil {
		t.Fatalf("GetApprovalEvidence: %v", err)
	}
	got, ok := resp.(openapi.GetApprovalEvidence200JSONResponse)
	if !ok || got.Kind != openapi.ApprovalEvidenceDetailKindNone || got.Gravitee != nil {
		t.Fatalf("ordinary evidence = %#v (%T)", resp, resp)
	}
}

func TestDecideApproval_aGraviteeAcceptanceWithoutEvidenceCannotBeApproved(t *testing.T) {
	t.Parallel()
	store := awaitingApprovalFor(t, "gravitee.apim.accept_subscription")
	ctx := inArea("cx", domain.RoleApprover)

	resp, err := NewServer(store, "test").DecideApproval(ctx,
		openapi.DecideApprovalRequestObject{
			RunId: "run-1", Body: &openapi.DecideApprovalJSONRequestBody{
				Approved: true, AtSeq: 2,
			},
		})
	if err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}
	if _, conflict := resp.(openapi.DecideApproval409ApplicationProblemPlusJSONResponse); !conflict {
		t.Fatalf("response = %T, want missing evidence refused", resp)
	}

	resp, err = NewServer(store, "test").DecideApproval(ctx,
		openapi.DecideApprovalRequestObject{
			RunId: "run-1", Body: &openapi.DecideApprovalJSONRequestBody{
				Approved: false, AtSeq: 2,
			},
		})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, ok := resp.(openapi.DecideApproval200JSONResponse); !ok {
		t.Fatalf("reject response = %T, want 200", resp)
	}
}

func awaitingApprovalFor(t *testing.T, tool domain.ToolID) *ledger.Memory {
	t.Helper()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	for _, step := range []domain.Step{
		{RunID: "run-1", Kind: domain.StepRunStarted, Scope: scope,
			AgentID: "triage", VersionID: "v1"},
		{RunID: "run-1", Kind: domain.StepApprovalRequested, Scope: scope,
			AgentID: "triage", VersionID: "v1",
			Payload: mustPayload(t, domain.ApprovalRequestedPayload{Tool: tool})},
	} {
		if _, err := store.Append(t.Context(), step); err != nil {
			t.Fatalf("append %s: %v", step.Kind, err)
		}
	}
	return store
}

func approvalWithGraviteeEvidence(
	t *testing.T, corruptDigest bool,
) (*Server, *ledger.Memory, *engine.MemoryContent) {
	t.Helper()
	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	ticketRef := domain.TicketRef{Key: "slack-ticket", Revision: 3}
	expires := "2026-09-13T12:00:00Z"
	snapshot := connectortools.GraviteeSnapshot{
		TicketRef: ticketRef, SubscriptionID: "sub-42", Status: "PENDING",
		Application: connectortools.GraviteeApplication{
			GraviteeResource:  connectortools.GraviteeResource{ID: "app-1", Name: "Portal"},
			PrimaryOwnerEmail: "dev@example.com",
		},
		API:          connectortools.GraviteeResource{ID: "checkout-api", Name: "Checkout"},
		Plan:         connectortools.GraviteeResource{ID: "plan-1", Name: "API keys"},
		PlanSecurity: "API_KEY", RequestedExpiration: &expires,
		RemoteCreatedAt: "2026-09-10T12:00:00Z", RemoteUpdatedAt: "2026-09-11T12:00:00Z",
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	ref, err := content.Put(t.Context(), "run-gravitee", 1, raw)
	if err != nil {
		t.Fatalf("put snapshot: %v", err)
	}
	digest := engine.ResultDigest(raw)
	if corruptDigest {
		digest = "sha256:another-snapshot"
	}
	steps := []domain.Step{
		{RunID: "run-gravitee", Kind: domain.StepRunStarted, Scope: scope,
			AgentID: "support", VersionID: "v1", OnBehalfOf: "svc_support",
			At: time.Date(2026, 9, 11, 11, 59, 0, 0, time.UTC),
			Payload: mustPayload(t, domain.RunStartedPayload{Ticket: &domain.TicketContext{
				Ref: ticketRef, RequestedBy: "usr_dev", AddressedBy: "support-bot",
			}})},
		{RunID: "run-gravitee", Kind: domain.StepApprovalRequested, Scope: scope,
			AgentID: "support", VersionID: "v1", OnBehalfOf: "svc_support",
			At: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC),
			Payload: mustPayload(t, domain.ApprovalRequestedPayload{
				Tool: "gravitee.apim.accept_subscription", Effect: domain.EffectWrite,
				Evidence: &domain.ApprovalEvidence{
					Kind:   connectortools.ApprovalEvidenceGraviteeSubscription,
					Ticket: ticketRef, Ref: ref, Digest: digest,
				},
			})},
	}
	for _, step := range steps {
		if _, err := store.Append(context.Background(), step); err != nil {
			t.Fatalf("append %s: %v", step.Kind, err)
		}
	}
	return NewServer(store, "test").WithContent(content), store, content
}
