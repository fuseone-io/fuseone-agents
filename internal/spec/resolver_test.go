package spec_test

import (
	"context"
	"io"
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

func TestResolve_fromTheRegistry_runsAnAgentNobodyPutOnDisk(t *testing.T) {
	registry := openRegistry(t)
	ctx := context.Background()

	// The whole authoring path — interview, editor, templates — publishes to
	// the registry and writes no file. Resolving only from disk meant every
	// one of those agents appeared on every screen, accepted a trigger, and
	// parked with spec_unresolved on its first turn (PRD DE-07).
	authored := published(t, definition)
	if err := registry.Publish(ctx, authored, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	providers := model.NewRegistry(nil)
	if err := providers.Register(model.Provider{
		Name: "openai", Kind: model.KindOpenAICompatible,
		BaseURL: "http://127.0.0.1:1", APIKey: "test",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	resolved, err := spec.NewResolver(registry, providers, nil).
		Resolve(ctx, authored.ID, authored.Version)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolved.Planner == nil {
		t.Error("resolved without a planner; the worker would park the run")
	}
	if resolved.Start.Budget.Micros != authored.Budget.Micros {
		t.Errorf("budget = %+v, want the published one", resolved.Start.Budget)
	}
	if len(resolved.Start.Pack.Tools()) != len(authored.Tools) {
		t.Errorf("pack = %v, want the published tools", resolved.Start.Pack.Tools())
	}
}

func TestResolve_rebuildsThePlannerWhenPricesChange(t *testing.T) {
	registry := openRegistry(t)
	ctx := context.Background()

	authored := published(t, definition)
	if err := registry.Publish(ctx, authored, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	providers := model.NewRegistry(nil)
	if err := providers.Register(model.Provider{
		Name: "openai", Kind: model.KindOpenAICompatible,
		BaseURL: "http://127.0.0.1:1", APIKey: "test",
		Prices: map[string]model.Prices{
			"test-model": {InputMicros: 1_000_000},
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	resolver := spec.NewResolver(registry, providers, nil)

	first, err := resolver.Resolve(ctx, authored.ID, authored.Version)
	if err != nil {
		t.Fatalf("Resolve first: %v", err)
	}
	if got := model.RateOf(first.Planner); got.InputMicros != 1_000_000 {
		t.Fatalf("first rate = %+v", got)
	}

	providers.SetPrices(map[string]map[string]model.Prices{
		"openai": {
			"test-model": {InputMicros: 9_000_000},
		},
	})
	second, err := resolver.Resolve(ctx, authored.ID, authored.Version)
	if err != nil {
		t.Fatalf("Resolve second: %v", err)
	}
	if got := model.RateOf(second.Planner); got.InputMicros != 9_000_000 {
		t.Fatalf("second rate = %+v, want refreshed planner", got)
	}
}

func TestResolve_replyLocale_reachesTheProviderWire(t *testing.T) {
	registry := openRegistry(t)
	ctx := context.Background()

	var captured atomic.Pointer[string]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		captured.Store(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5",
			"content":[{"type":"tool_use","id":"t1","name":"crm.lookup","input":{}}],
			"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	t.Cleanup(srv.Close)

	def := strings.Replace(definition, "provider: openai", "provider: anthropic", 1)
	def = strings.Replace(def, "model: test-model", "model: claude-opus-5\nreply_locale: pt-BR", 1)
	authored := published(t, def)
	if err := registry.Publish(ctx, authored, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	providers := model.NewRegistry(nil)
	if err := providers.Register(model.Provider{
		Name: "anthropic", Kind: model.KindAnthropic, BaseURL: srv.URL, APIKey: "test",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	resolved, err := spec.NewResolver(registry, providers, nil).
		Resolve(ctx, authored.ID, authored.Version)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// The definition is the whole journey: what the author wrote has to be
	// what the provider receives, or the field is decoration (#156).
	_, _ = resolved.Planner.Plan(ctx, engine.PlanInput{
		Transcript: []engine.Turn{{Kind: engine.TurnInput, Text: "hi"}},
		Tools:      []domain.ToolID{"crm.lookup"},
	})
	body := captured.Load()
	if body == nil {
		t.Fatal("the provider was never called")
	}
	if !strings.Contains(*body, "including the finish summary, in pt-BR") {
		t.Errorf("request carries no language pin; the published locale never reached the wire")
	}
}

func TestResolve_cachePrefixTTL_reachesTheProviderWire(t *testing.T) {
	registry := openRegistry(t)
	ctx := context.Background()

	var captured atomic.Pointer[string]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		captured.Store(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5",
			"content":[{"type":"tool_use","id":"t1","name":"crm.lookup","input":{}}],
			"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	t.Cleanup(srv.Close)

	def := strings.Replace(definition, "provider: openai", "provider: anthropic", 1)
	def = strings.Replace(def, "model: test-model", "model: claude-opus-5\ncache_prefix_ttl: 1h", 1)
	authored := published(t, def)
	if err := registry.Publish(ctx, authored, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	providers := model.NewRegistry(nil)
	if err := providers.Register(model.Provider{
		Name: "anthropic", Kind: model.KindAnthropic, BaseURL: srv.URL, APIKey: "test",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	resolved, err := spec.NewResolver(registry, providers, nil).
		Resolve(ctx, authored.ID, authored.Version)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// The definition is the whole journey: what the author wrote has to be
	// what the provider receives, or the field is decoration.
	_, _ = resolved.Planner.Plan(ctx, engine.PlanInput{
		Transcript: []engine.Turn{{Kind: engine.TurnInput, Text: "hi"}},
		Tools:      []domain.ToolID{"crm.lookup"},
	})
	body := captured.Load()
	if body == nil {
		t.Fatal("the provider was never called")
	}
	if !strings.Contains(*body, `"ttl":"1h"`) {
		t.Errorf("request carries no 1h ttl; the published window never reached the wire")
	}
}

func TestResolve_agentIDRidesAsThePromptCacheKey(t *testing.T) {
	registry := openRegistry(t)
	ctx := context.Background()

	var captured atomic.Pointer[string]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		captured.Store(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	t.Cleanup(srv.Close)

	authored := published(t, definition)
	if err := registry.Publish(ctx, authored, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	providers := model.NewRegistry(nil)
	if err := providers.Register(model.Provider{
		Name: "openai", Kind: model.KindOpenAICompatible,
		BaseURL: srv.URL, APIKey: "test", SupportsPromptCacheKey: true,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	resolved, err := spec.NewResolver(registry, providers, nil).
		Resolve(ctx, authored.ID, authored.Version)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	_, _ = resolved.Planner.Plan(ctx, engine.PlanInput{
		Transcript: []engine.Turn{{Kind: engine.TurnInput, Text: "hi"}},
		Tools:      []domain.ToolID{"crm.lookup"},
	})
	body := captured.Load()
	if body == nil {
		t.Fatal("the provider was never called")
	}
	if !strings.Contains(*body, `"prompt_cache_key":"triage"`) {
		t.Errorf("request carries no agent cache key; the resolver stopped naming it")
	}
}
