package httpapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// The fallback pair is part of the definition: published from the form,
// handed back to the editor, and shown on a run that used it.

type configuredProviders map[string]bool

func (k configuredProviders) Configured(name string) bool { return k[name] }

func withFallbackPair(provider, model string) func(*openapi.AgentDefinition) {
	return func(d *openapi.AgentDefinition) {
		d.Fallback = &openapi.ModelPair{Provider: provider, Model: model}
	}
}

func TestPublishAgent_fallbackSurvivesTheConsolePath(t *testing.T) {
	t.Parallel()
	pub := newPublisher()

	_, err := publishServer(t, pub).WithProviderNames(configuredProviders{"openai": true, "backup": true}).
		PublishAgent(inArea("cx", domain.RoleAuthor), openapi.PublishAgentRequestObject{
			AgentId: "triage", Body: definition(withFallbackPair("backup", "backup-model")),
		})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}

	want := domain.ModelPair{Provider: "backup", Model: "backup-model"}
	if len(pub.published) != 1 || pub.published[0].Fallback != want {
		t.Fatalf("published = %+v, want the fallback pair to survive the form", pub.published)
	}
}

func TestPublishAgent_aFallbackProviderNobodyConfigured_isRefused(t *testing.T) {
	t.Parallel()
	pub := newPublisher()

	// Reviewed at publish, because the moment it would otherwise be found out
	// is the outage it exists for.
	resp, err := publishServer(t, pub).WithProviderNames(configuredProviders{"openai": true}).
		PublishAgent(inArea("cx", domain.RoleAuthor), openapi.PublishAgentRequestObject{
			AgentId: "triage", Body: definition(withFallbackPair("backup", "backup-model")),
		})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	bad, ok := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("response = %T, want a refusal", resp)
	}
	if !strings.Contains(valueOr(bad.Detail), `"backup"`) {
		t.Errorf("detail = %q, want it to name the provider", valueOr(bad.Detail))
	}
	if len(pub.published) != 0 {
		t.Error("published a fallback that cannot be asked")
	}
}

func TestPublishAgent_aFallbackIdenticalToTheDeclaredPair_isRefused(t *testing.T) {
	t.Parallel()
	pub := newPublisher()

	resp, err := publishServer(t, pub).WithProviderNames(configuredProviders{"openai": true}).
		PublishAgent(inArea("cx", domain.RoleAuthor), openapi.PublishAgentRequestObject{
			AgentId: "triage", Body: definition(withFallbackPair("openai", "gpt-test")),
		})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	if _, ok := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want a refusal", resp)
	}
}

func TestAgentFrom_handsTheFallbackBackToTheEditor(t *testing.T) {
	t.Parallel()

	got := agentFrom(domain.AgentSummary{
		ID: "triage", Fallback: domain.ModelPair{Provider: "backup", Model: "backup-model"},
	})
	if got.Fallback == nil || got.Fallback.Provider != "backup" || got.Fallback.Model != "backup-model" {
		t.Fatalf("Fallback = %+v, want the pair: what a read omits, a republish deletes", got.Fallback)
	}
	if none := agentFrom(domain.AgentSummary{ID: "triage"}); none.Fallback != nil {
		t.Errorf("Fallback = %+v, want absent when none is declared", none.Fallback)
	}
}

func TestGetRun_aRunThatFellBack_saysSo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := ledger.NewMemory()
	at := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

	for i, step := range []domain.Step{
		{Kind: domain.StepRunStarted},
		{Kind: domain.StepModelFallback, Payload: mustPayload(t, domain.ModelFallbackPayload{
			From:    domain.ModelPair{Provider: "anthropic", Model: "claude-opus-5"},
			To:      domain.ModelPair{Provider: "backup", Model: "backup-model"},
			Failure: domain.FailureSummary{Code: "model_provider_overloaded", Retryable: true},
		})},
	} {
		step.RunID, step.Scope, step.AgentID, step.VersionID = "run-cx", domain.Scope{Company: "acme", Area: "cx"}, "triage", "v1"
		step.At = at.Add(time.Duration(i) * time.Second)
		if _, err := store.Append(ctx, step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}

	resp, err := NewServer(store, "test").
		GetRun(inArea("cx", domain.RoleAuthor), openapi.GetRunRequestObject{RunId: "run-cx"})
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	got := resp.(openapi.GetRun200JSONResponse).Fallback
	if got == nil {
		t.Fatal("a run that fell back reads like any other")
	}
	if got.From.Provider != "anthropic" || got.To.Provider != "backup" || got.To.Model != "backup-model" ||
		got.Code != "model_provider_overloaded" || got.AtSeq != 2 {
		t.Errorf("fallback = %+v", *got)
	}
}

func TestGetRun_aRunThatNeverFellBack_carriesNoMark(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := ledger.NewMemory()
	if _, err := store.Append(ctx, domain.Step{
		RunID: "run-cx", Kind: domain.StepRunStarted, Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: "v1", At: time.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resp, err := NewServer(store, "test").
		GetRun(inArea("cx", domain.RoleAuthor), openapi.GetRunRequestObject{RunId: "run-cx"})
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got := resp.(openapi.GetRun200JSONResponse).Fallback; got != nil {
		t.Errorf("Fallback = %+v, want none", *got)
	}
}
