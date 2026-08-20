package spec_test

import (
	"context"
	"testing"

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
