package spec_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/spec"
)

/*
A run plans only on a provider its scope reaches (NT-019, phase 3 readers).

The resolver caches a planner per version and pair, and the registry's revision
is what retires one. A provider that moves from the installation to an area with
every other field the same must retire the planner a run of another area was
using: otherwise that run keeps planning on a provider, and a key, its scope no
longer reaches.
*/

type oneDefinition struct{ s spec.Spec }

func (d oneDefinition) Get(context.Context, domain.AgentID, domain.VersionID) (spec.Spec, error) {
	return d.s, nil
}

func llmAt(scope domain.Scope) model.Provider {
	return model.Provider{Name: "openai", Kind: model.KindOpenAICompatible, BaseURL: "http://127.0.0.1:1", APIKey: "k", Scope: scope}
}

func claimAt(s spec.Spec, scope domain.Scope) domain.Claim {
	return domain.Claim{RunID: "run-scope", AgentID: s.ID, VersionID: s.Version, Scope: scope}
}

func TestResolve_aPlannerCachedForTheInstallation_isNotReusedOnceItsProviderMovesAway(t *testing.T) {
	t.Parallel()
	authored := published(t, definition)
	finance := domain.Scope{Company: "acme", Area: "finance"}
	cx := domain.Scope{Company: "acme", Area: "cx"}
	providers := model.NewRegistry(nil)
	providers.SetConfigured([]model.Provider{llmAt(domain.Scope{Company: domain.Installation})},
		model.Claim{Name: "openai", Scope: domain.Scope{Company: domain.Installation}})
	resolver := spec.NewResolver(oneDefinition{authored}, providers, nil)

	if _, err := resolver.Resolve(context.Background(), claimAt(authored, finance)); err != nil {
		t.Fatalf("Resolve at acme/finance while the provider is the installation's: %v", err)
	}
	providers.SetConfigured([]model.Provider{llmAt(cx)}, model.Claim{Name: "openai", Scope: cx})

	if _, err := resolver.Resolve(context.Background(), claimAt(authored, finance)); err == nil {
		t.Fatal("a run of acme/finance reused the planner of a provider now acme/cx's")
	}
	if _, err := resolver.Resolve(context.Background(), claimAt(authored, cx)); err != nil {
		t.Fatalf("Resolve at acme/cx: %v", err)
	}
}

// The same version run from two scopes keeps one planner per scope: a run of
// acme/cx must not be answered from a planner cached for another scope.
func TestResolve_aRunOfAnotherScope_isJudgedAtItsOwn(t *testing.T) {
	t.Parallel()
	authored := published(t, definition)
	cx := domain.Scope{Company: "acme", Area: "cx"}
	providers := model.NewRegistry(nil)
	providers.SetConfigured([]model.Provider{llmAt(cx)}, model.Claim{Name: "openai", Scope: cx})
	resolver := spec.NewResolver(oneDefinition{authored}, providers, nil)

	if _, err := resolver.Resolve(context.Background(), claimAt(authored, cx)); err != nil {
		t.Fatalf("Resolve at acme/cx: %v", err)
	}
	if _, err := resolver.Resolve(context.Background(), claimAt(authored, domain.Scope{Company: "acme", Area: "ops"})); err == nil {
		t.Fatal("a run of acme/ops planned on acme/cx's provider from the cache")
	}
}

// claimOf is a claim for a version, run from acme's area the definition names.
func claimOf(s spec.Spec) domain.Claim {
	return domain.Claim{RunID: "run-test", AgentID: s.ID, VersionID: s.Version, Scope: domain.Scope{Company: "acme", Area: s.Area}}
}
