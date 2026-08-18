package httpapi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// The console asks for a step's content when somebody opens the step — an
// approval it has to show the arguments for, a result someone is checking.
// The trail itself never carries them: it is read constantly, by many people,
// and the content behind it routinely carries personal data (AU-04).

func runWithArguments(t *testing.T) (*ledger.Memory, *engine.MemoryContent, string) {
	t.Helper()
	ctx := context.Background()
	store, content := ledger.NewMemory(), engine.NewMemoryContent()

	args := []byte(`{"email":"cliente@exemplo.com.br"}`)
	ref, err := content.Put(ctx, "run-cx", 2, args)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	for _, step := range []domain.Step{
		{RunID: "run-cx", Kind: domain.StepRunStarted, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v1", At: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)},
		{RunID: "run-cx", Kind: domain.StepApprovalRequested, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v1", At: time.Date(2026, 8, 11, 12, 0, 1, 0, time.UTC),
			Payload: mustPayload(t, domain.ApprovalRequestedPayload{
				Tool: "crm.note", Rule: "policy", ArgsRef: ref, ArgsDigest: "abc123",
			})},
	} {
		if _, err := store.Append(ctx, step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}
	return store, content, string(args)
}

func mustPayload(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestGetStepContent_pendingApproval_returnsTheProposedArguments(t *testing.T) {
	t.Parallel()

	store, content, args := runWithArguments(t)
	resp, err := NewServer(store, "test").WithContent(content).
		GetStepContent(inArea("cx", domain.RoleAuthor),
			openapi.GetStepContentRequestObject{RunId: "run-cx", Seq: 2})
	if err != nil {
		t.Fatalf("GetStepContent: %v", err)
	}

	got, ok := resp.(openapi.GetStepContent200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the content", resp)
	}
	if got.Content != args {
		t.Errorf("content = %q, want %q", got.Content, args)
	}
	if got.Digest != "abc123" {
		t.Errorf("digest = %q, want the one the chain sealed", got.Digest)
	}
}

func TestGetStepContent_finishedRun_returnsTheClosingAnswer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	answer := "Refunded R$ 88,21 to Maria Silva."
	ref, err := content.Put(ctx, "run-cx", 2, []byte(answer))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	for _, step := range []domain.Step{
		{RunID: "run-cx", Kind: domain.StepRunStarted, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v1", At: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)},
		{RunID: "run-cx", Kind: domain.StepRunFinished, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v1", At: time.Date(2026, 8, 11, 12, 0, 1, 0, time.UTC),
			Payload: mustPayload(t, domain.RunFinishedPayload{
				OutcomeRef: ref, OutcomeDigest: "sha256:answer",
			})},
	} {
		if _, err := store.Append(ctx, step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}

	resp, err := NewServer(store, "test").WithContent(content).
		GetStepContent(inArea("cx", domain.RoleAuthor),
			openapi.GetStepContentRequestObject{RunId: "run-cx", Seq: 2})
	if err != nil {
		t.Fatalf("GetStepContent: %v", err)
	}

	got, ok := resp.(openapi.GetStepContent200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the answer content", resp)
	}
	if got.Content != answer {
		t.Errorf("content = %q, want %q", got.Content, answer)
	}
	if got.Digest != "sha256:answer" {
		t.Errorf("digest = %q, want the one the chain sealed", got.Digest)
	}
}

func TestGetStepContent_inAnotherArea_readsAsAbsent(t *testing.T) {
	t.Parallel()

	store, content, _ := runWithArguments(t)
	resp, err := NewServer(store, "test").WithContent(content).
		GetStepContent(inArea("marketing", domain.RoleAuthor),
			openapi.GetStepContentRequestObject{RunId: "run-cx", Seq: 2})
	if err != nil {
		t.Fatalf("GetStepContent: %v", err)
	}
	if _, absent := resp.(openapi.GetStepContent404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

func TestGetStepContent_stepThatReferencesNothing_readsAsAbsent(t *testing.T) {
	t.Parallel()

	// Not an empty body: a caller that cannot tell "no content here" from
	// "content that is empty" will render an approval with blank arguments.
	store, content, _ := runWithArguments(t)
	resp, err := NewServer(store, "test").WithContent(content).
		GetStepContent(inArea("cx", domain.RoleAuthor),
			openapi.GetStepContentRequestObject{RunId: "run-cx", Seq: 1})
	if err != nil {
		t.Fatalf("GetStepContent: %v", err)
	}
	if _, absent := resp.(openapi.GetStepContent404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

// A run waiting on a person has to say when it stopped and what it wants to
// do. Both were missing from the run's own projection while the inbox had
// them, so the detail screen showed an approval requested in the year one, for
// a call with no effect.

func TestGetRun_awaitingApproval_saysWhenItStoppedAndWhatItWillDo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := ledger.NewMemory()

	asked := time.Date(2026, 8, 11, 14, 24, 59, 0, time.UTC)
	for _, step := range []domain.Step{
		{RunID: "run-cx", Kind: domain.StepRunStarted, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v1", At: asked.Add(-time.Minute)},
		{RunID: "run-cx", Kind: domain.StepApprovalRequested, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v1", At: asked,
			Payload: mustPayload(t, domain.ApprovalRequestedPayload{
				Tool: "crm.reply", Rule: "taint", Effect: domain.EffectWrite,
			})},
	} {
		if _, err := store.Append(ctx, step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}

	resp, err := NewServer(store, "test").
		GetRun(inArea("cx", domain.RoleApprover), openapi.GetRunRequestObject{RunId: "run-cx"})
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	run := resp.(openapi.GetRun200JSONResponse)

	pending := run.PendingApproval
	if pending == nil {
		t.Fatal("the run does not report the approval it is waiting on")
	}
	if !pending.RequestedAt.Equal(asked) {
		t.Errorf("requestedAt = %s, want %s", pending.RequestedAt, asked)
	}
	if pending.Effect == nil || *pending.Effect != "write" {
		t.Errorf("effect = %v, want write — an approver decides on what it does to the world", pending.Effect)
	}
}

func TestGetRun_parkedProviderFailureCarriesTheStableCause(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := ledger.NewMemory()

	at := time.Date(2026, 8, 18, 17, 6, 13, 0, time.UTC)
	for _, step := range []domain.Step{
		{RunID: "run-cx", Kind: domain.StepRunStarted, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v1", At: at.Add(-time.Minute)},
		{RunID: "run-cx", Kind: domain.StepParked, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v1", At: at,
			Payload: mustPayload(t, domain.ParkedPayload{
				Reason:   "model_provider_overloaded",
				Attempts: 5,
				Failure: &domain.FailureSummary{
					Code:      "model_provider_overloaded",
					Provider:  "anthropic",
					Status:    529,
					RequestID: "req_011CeAaYZkdUe63yaSu5CxCX",
					Retryable: true,
				},
			})},
	} {
		if _, err := store.Append(ctx, step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}

	resp, err := NewServer(store, "test").
		GetRun(inArea("cx", domain.RoleAuthor), openapi.GetRunRequestObject{RunId: "run-cx"})
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	run := resp.(openapi.GetRun200JSONResponse)
	if run.Failure == nil {
		t.Fatal("run has no failure summary")
	}
	if run.Failure.Code != "model_provider_overloaded" || valueOr(run.Failure.Provider) != "anthropic" ||
		valueOr(run.Failure.Status) != 529 || valueOr(run.Failure.RequestId) != "req_011CeAaYZkdUe63yaSu5CxCX" ||
		valueOr(run.Failure.Retryable) != true {
		t.Errorf("failure = %+v, want the stable parked provider cause", *run.Failure)
	}
}
