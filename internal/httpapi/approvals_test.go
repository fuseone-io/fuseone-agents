package httpapi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// A decision that does not say who made it is the one record this product
// exists to keep, kept empty.

type fixedAt struct{ t time.Time }

func (f fixedAt) Now() time.Time { return f.t }

func awaitingApproval(t *testing.T, at time.Time) *ledger.Memory {
	t.Helper()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}

	for _, step := range []domain.Step{
		{RunID: "run-1", Kind: domain.StepRunStarted, Scope: scope,
			AgentID: "triage", VersionID: "v1", At: at.Add(-time.Minute)},
		{RunID: "run-1", Kind: domain.StepApprovalRequested, Scope: scope,
			AgentID: "triage", VersionID: "v1", At: at,
			Payload: mustPayload(t, domain.ApprovalRequestedPayload{Tool: "crm.reply", Rule: "taint"})},
	} {
		if _, err := store.Append(context.Background(), step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}
	return store
}

func TestDecideApproval_recordsWhoDecidedAndWhen(t *testing.T) {
	t.Parallel()

	asked := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	store := awaitingApproval(t, asked)
	decided := asked.Add(3 * time.Minute)

	if _, err := NewServer(store, "test").WithClock(fixedAt{t: decided}).
		DecideApproval(inArea("cx", domain.RoleApprover), openapi.DecideApprovalRequestObject{
			RunId: "run-1",
			Body:  &openapi.DecideApprovalJSONRequestBody{Approved: true, AtSeq: 2},
		}); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}

	steps, err := store.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	last := steps[len(steps)-1]

	var payload domain.ApprovalDecidedPayload
	if err := json.Unmarshal(last.Payload, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Without this the trail says an action was authorised and cannot say by
	// whom — the product's whole promise, kept empty.
	if payload.By == "" {
		t.Error("the decision does not record who made it")
	}
	// Stamped with the moment it was made. Copying the previous step's time
	// makes every decision look instantaneous and erases how long a person
	// took, which is what the human queue is measured by.
	if !last.At.Equal(decided) {
		t.Errorf("decided at %s, want %s", last.At, decided)
	}
}

func TestDecideApproval_clockBehindTheLastStep_doesNotWalkTheChainBackwards(t *testing.T) {
	t.Parallel()

	// Two machines, two clocks. A step stamped before the one it seals would
	// make the trail read out of order for a reason nobody could diagnose.
	asked := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	store := awaitingApproval(t, asked)

	if _, err := NewServer(store, "test").WithClock(fixedAt{t: asked.Add(-time.Hour)}).
		DecideApproval(inArea("cx", domain.RoleApprover), openapi.DecideApprovalRequestObject{
			RunId: "run-1",
			Body:  &openapi.DecideApprovalJSONRequestBody{Approved: false, AtSeq: 2},
		}); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}

	steps, _ := store.Read(context.Background(), "run-1", domain.FirstSeq)
	last := steps[len(steps)-1]
	if last.At.Before(asked) {
		t.Errorf("the decision is stamped %s, before the request at %s", last.At, asked)
	}
}
