package spec_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/model"
)

/*
An experiment resolves the version under its configuration (NT-015 § Experiments).

Any non-empty configuration drops the declared fallback, a partial one included: the
experiment asked about one model, and a fallback that switched to another would report
model X while measuring something else. An empty configuration is the version as
published and keeps its fallback. The model resolves through the same registry a
published agent does, so an experiment cannot reach a provider this installation has not
configured.
*/

// asked is what a provider was asked: the model and the reasoning effort.
type asked struct {
	mu    sync.Mutex
	calls []struct{ model, effort string }
}

func (a *asked) last() (string, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.calls) == 0 {
		return "", ""
	}
	c := a.calls[len(a.calls)-1]
	return c.model, c.effort
}

func effortServer(t *testing.T, into *asked) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model  string `json:"model"`
			Effort string `json:"reasoning_effort"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		into.mu.Lock()
		into.calls = append(into.calls, struct{ model, effort string }{body.Model, body.Effort})
		into.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": "done"}}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 2},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

const experimentDefinition = `---
id: triage
name: Ticket triage
area: cx
provider: openai
model: test-model
effort: high
fallback: {provider: backup, model: backup-model}
tools:
  - crm.lookup
budget:
  micros: 500000
  steps: 60
---

Read the ticket and classify it.
`

type experimentProviders struct {
	registry        *model.Registry
	primary, backup *asked
}

func newExperimentProviders(t *testing.T) experimentProviders {
	t.Helper()
	p := experimentProviders{registry: model.NewRegistry(nil), primary: &asked{}, backup: &asked{}}
	for name, into := range map[string]*asked{"openai": p.primary, "backup": p.backup} {
		if err := p.registry.Register(model.Provider{
			Name: name, Kind: model.KindOpenAICompatible, BaseURL: effortServer(t, into),
			APIKey: "k", SupportsReasoningEffort: true,
		}); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	return p
}

func plan(t *testing.T, planner engine.Planner) {
	t.Helper()
	if _, err := planner.Plan(context.Background(), engine.PlanInput{
		Budget: domain.Budget{Micros: 500_000},
	}); err != nil {
		t.Fatalf("Plan: %v", err)
	}
}

func TestResolveExperiment_aNonEmptyConfiguration_plansOnItAndDropsTheFallback(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                  string
		config                domain.ExperimentConfig
		provider              string
		wantModel, wantEffort string
	}{
		{"only effort", domain.ExperimentConfig{Effort: "low"}, "openai", "test-model", "low"},
		{"another model", domain.ExperimentConfig{Model: "other-model"}, "openai", "other-model", "high"},
		{"another provider", domain.ExperimentConfig{Provider: "backup", Model: "b-2"}, "backup", "b-2", "high"},
		// Every field is optional and an absent one is the version's own (NT-015).
		{"only a provider", domain.ExperimentConfig{Provider: "backup"}, "backup", "test-model", "high"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			providers := newExperimentProviders(t)
			s, resolver := resolveLocal(t, providers.registry, experimentDefinition)

			resolved, err := resolver.ResolveExperiment(context.Background(), s.ID, s.Version, c.config)
			if err != nil {
				t.Fatalf("ResolveExperiment: %v", err)
			}
			if resolved.Fallback != nil {
				t.Errorf("fallback = %+v, want none: the experiment asked about one model", resolved.Fallback)
			}
			plan(t, resolved.Planner)
			into := map[string]*asked{"openai": providers.primary, "backup": providers.backup}[c.provider]
			if m, e := into.last(); m != c.wantModel || e != c.wantEffort {
				t.Errorf("%s was asked %s at %q, want %s at %q", c.provider, m, e, c.wantModel, c.wantEffort)
			}
		})
	}
}

func TestResolveExperiment_anEmptyConfiguration_isTheVersionAsPublished(t *testing.T) {
	t.Parallel()
	providers := newExperimentProviders(t)
	s, resolver := resolveLocal(t, providers.registry, experimentDefinition)

	resolved, err := resolver.ResolveExperiment(context.Background(), s.ID, s.Version, domain.ExperimentConfig{})
	if err != nil {
		t.Fatalf("ResolveExperiment: %v", err)
	}
	if resolved.Fallback == nil ||
		resolved.Fallback.To != (domain.ModelPair{Provider: "backup", Model: "backup-model"}) {
		t.Errorf("fallback = %+v, want the version's declared one", resolved.Fallback)
	}
	plan(t, resolved.Planner)
	if m, e := providers.primary.last(); m != "test-model" || e != "high" {
		t.Errorf("asked %s at %q, want the version's own test-model at high", m, e)
	}
}

// An experiment's effort is its own planner. Cached under the version and pair alone, the
// run that resolved next, real or not, would plan at the experiment's effort.
func TestResolveExperiment_anotherEffort_doesNotLeakIntoTheVersionsPlanner(t *testing.T) {
	t.Parallel()
	providers := newExperimentProviders(t)
	s, resolver := resolveLocal(t, providers.registry, experimentDefinition)

	if _, err := resolver.ResolveExperiment(context.Background(), s.ID, s.Version,
		domain.ExperimentConfig{Effort: "low"}); err != nil {
		t.Fatalf("ResolveExperiment: %v", err)
	}
	resolved, err := resolver.Resolve(context.Background(), s.ID, s.Version)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	plan(t, resolved.Planner)
	if _, e := providers.primary.last(); e != "high" {
		t.Errorf("the published version planned at %q, want its own high", e)
	}
}

func TestResolveExperiment_aModelTheInstallationDoesNotServe_isRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		config domain.ExperimentConfig
	}{
		{"a provider nobody configured", domain.ExperimentConfig{Provider: "elsewhere", Model: "x"}},
		{"only a provider, nobody configured", domain.ExperimentConfig{Provider: "elsewhere"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			providers := newExperimentProviders(t)
			s, resolver := resolveLocal(t, providers.registry, experimentDefinition)

			if _, err := resolver.ResolveExperiment(context.Background(), s.ID, s.Version, c.config); err == nil {
				t.Error("resolved an experiment on a model this installation does not serve")
			}
		})
	}
}
