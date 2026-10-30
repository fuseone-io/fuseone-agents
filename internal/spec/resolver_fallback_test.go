package spec_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/spec"
)

// chatServer answers chat completions and remembers which model was asked.
func chatServer(t *testing.T, asked *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		asked.Store(body.Model)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": "done"}}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 2},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

const fallbackDefinition = `---
id: triage
name: Ticket triage
area: cx
provider: openai
model: test-model
fallback: {provider: backup, model: backup-model}
tools:
  - crm.lookup
budget:
  micros: 500000
  steps: 60
---

Read the ticket and classify it.
`

type twoProviders struct {
	registry              *model.Registry
	primary, fallback     *atomic.Value
	primaryURL, backupURL string
}

func newTwoProviders(t *testing.T) twoProviders {
	t.Helper()
	p := twoProviders{registry: model.NewRegistry(nil), primary: &atomic.Value{}, fallback: &atomic.Value{}}
	p.primaryURL = chatServer(t, p.primary).URL
	p.backupURL = chatServer(t, p.fallback).URL
	for _, provider := range []model.Provider{
		{Name: "openai", Kind: model.KindOpenAICompatible, BaseURL: p.primaryURL, APIKey: "k",
			Prices: map[string]model.Prices{"test-model": {InputMicros: 1_000_000}}},
		{Name: "backup", Kind: model.KindOpenAICompatible, BaseURL: p.backupURL, APIKey: "k",
			Prices: map[string]model.Prices{"backup-model": {InputMicros: 7_000_000}}},
	} {
		if err := p.registry.Register(provider); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	return p
}

func resolveLocal(t *testing.T, providers *model.Registry, source string) (spec.Spec, *spec.Resolver) {
	t.Helper()
	store := spec.NewStore()
	s, err := spec.Parse("triage.agent.md", []byte(source))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := store.Publish(s); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return s, spec.NewResolver(spec.Local(store), providers, nil)
}

func TestResolve_aDeclaredFallback_isHandedToTheWorker(t *testing.T) {
	t.Parallel()

	providers := newTwoProviders(t)
	s, resolver := resolveLocal(t, providers.registry, fallbackDefinition)

	resolved, err := resolver.Resolve(context.Background(), s.ID, s.Version)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Fallback == nil {
		t.Fatal("resolved without the declared fallback")
	}
	if resolved.Fallback.From != (domain.ModelPair{Provider: "openai", Model: "test-model"}) ||
		resolved.Fallback.To != (domain.ModelPair{Provider: "backup", Model: "backup-model"}) {
		t.Errorf("fallback = %+v", resolved.Fallback)
	}
}

func TestResolve_theFallbackPlanner_asksTheFallbackPairAndBillsUnderIt(t *testing.T) {
	t.Parallel()

	providers := newTwoProviders(t)
	s, resolver := resolveLocal(t, providers.registry, fallbackDefinition)
	resolved, err := resolver.Resolve(context.Background(), s.ID, s.Version)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	proposal, err := resolved.Fallback.Planner.Plan(context.Background(), engine.PlanInput{
		Budget: domain.Budget{Micros: 500_000}, Tools: []domain.ToolID{"crm.lookup"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got, _ := providers.fallback.Load().(string); got != "backup-model" {
		t.Errorf("fallback server was asked for %q, want backup-model", got)
	}
	if providers.primary.Load() != nil {
		t.Error("the declared provider was asked by the fallback planner")
	}
	// The planned step records what the proposal names, and FinOps groups by
	// it: the fallback's spend must land on the fallback's pair and rate.
	if proposal.Provider != "backup" || proposal.Model != "backup-model" {
		t.Errorf("proposal pair = %s/%s, want backup/backup-model", proposal.Provider, proposal.Model)
	}
	if proposal.Cost.Micros != 70 {
		t.Errorf("cost = %d micros, want the fallback's rate (10 tokens at 7/token-million → 70)", proposal.Cost.Micros)
	}
}

func TestResolve_theCacheKeepsTheTwoPairsApart(t *testing.T) {
	t.Parallel()

	// One planner per version was the cache's whole key. Keyed that way, the
	// fallback resolves to whichever pair was built first.
	providers := newTwoProviders(t)
	s, resolver := resolveLocal(t, providers.registry, fallbackDefinition)
	for range 2 {
		resolved, err := resolver.Resolve(context.Background(), s.ID, s.Version)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got := model.RateOf(resolved.Planner); got.InputMicros != 1_000_000 {
			t.Errorf("primary rate = %+v", got)
		}
		if got := model.RateOf(resolved.Fallback.Planner); got.InputMicros != 7_000_000 {
			t.Errorf("fallback rate = %+v, want the fallback pair's", got)
		}
	}
}

func TestResolve_noFallbackDeclared_resolvesNone(t *testing.T) {
	t.Parallel()

	providers := newTwoProviders(t)
	plain := strings.Replace(fallbackDefinition, "fallback: {provider: backup, model: backup-model}\n", "", 1)
	s, resolver := resolveLocal(t, providers.registry, plain)

	resolved, err := resolver.Resolve(context.Background(), s.ID, s.Version)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Fallback != nil {
		t.Errorf("Fallback = %+v, want none", resolved.Fallback)
	}
}

func TestResolve_aFallbackThisWorkerCannotBuild_leavesTheDeclaredPairRunning(t *testing.T) {
	t.Parallel()

	// A fallback provider removed after publishing must not stop a run that
	// never needed it. It fails only when asked.
	providers := newTwoProviders(t)
	s, resolver := resolveLocal(t, providers.registry,
		strings.Replace(fallbackDefinition, "provider: backup", "provider: gone", 1))

	resolved, err := resolver.Resolve(context.Background(), s.ID, s.Version)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Planner == nil || resolved.Fallback == nil {
		t.Fatalf("resolved = %+v, want both sides", resolved)
	}
	if _, err := resolved.Fallback.Planner.Plan(context.Background(), engine.PlanInput{}); err == nil {
		t.Error("an unbuildable fallback planned anyway")
	}
}
