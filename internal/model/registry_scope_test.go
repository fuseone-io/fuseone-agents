package model_test

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
)

/*
A provider is found only from inside its scope (NT-019, phase 3, readers).

The registry was keyed by name alone, so once an area stores a provider with its
own key, a run of any other area naming it would be handed that key. Every
lookup now takes the scope it will run in, and a provider whose scope does not
contain it is not found: not planned, not usable, not configured.
*/

var (
	installation = domain.Scope{Company: domain.Installation}
	cx           = domain.Scope{Company: "acme", Area: "cx"}
	finance      = domain.Scope{Company: "acme", Area: "finance"}
)

func areaProvider(name string, scope domain.Scope) model.Provider {
	return model.Provider{Name: name, Kind: model.KindOpenAICompatible, BaseURL: "http://127.0.0.1:9", Scope: scope}
}

func TestPlanner_aProviderOfAnArea_isFoundOnlyFromInsideIt(t *testing.T) {
	t.Parallel()

	r := model.NewRegistry(nil)
	r.SetConfigured([]model.Provider{areaProvider("cx-llm", cx)}, model.Claim{Name: "cx-llm", Scope: cx})

	if _, err := r.Planner("cx-llm", cx, model.Config{Model: "m"}, nil); err != nil {
		t.Fatalf("Planner from acme/cx: %v", err)
	}
	if _, err := r.Planner("cx-llm", finance, model.Config{Model: "m"}, nil); err == nil {
		t.Fatal("Planner from acme/finance found acme/cx's provider")
	}
}

// The installation's providers are every scope's, as before.
func TestPlanner_aProviderOfTheInstallation_isFoundFromAnyScope(t *testing.T) {
	t.Parallel()

	r := model.NewRegistry(nil)
	r.SetConfigured([]model.Provider{areaProvider("shared", installation)}, model.Claim{Name: "shared", Scope: installation})
	for _, at := range []domain.Scope{cx, finance, {Company: "globex"}} {
		if _, err := r.Planner("shared", at, model.Config{Model: "m"}, nil); err != nil {
			t.Errorf("Planner from %s: %v", at, err)
		}
	}
}

// A provider read from a row always carries its scope. One that arrives with
// none reaches nothing, rather than every scope.
func TestPlanner_aConfiguredProviderWithNoScope_isFoundNowhere(t *testing.T) {
	t.Parallel()

	r := model.NewRegistry(nil)
	r.SetConfigured([]model.Provider{areaProvider("unplaced", domain.Scope{})}, model.Claim{Name: "unplaced"})
	if _, err := r.Planner("unplaced", cx, model.Config{Model: "m"}, nil); err == nil {
		t.Fatal("a provider with no scope was found")
	}
}

// The refusal names the provider asked for and nothing else: listing what is
// configured would name other areas' providers to a run that may not see them.
func TestPlanner_refusal_namesNoOtherProvider(t *testing.T) {
	t.Parallel()

	r := model.NewRegistry(nil)
	r.SetConfigured([]model.Provider{areaProvider("cx-llm", cx), areaProvider("ops-llm", domain.Scope{Company: "acme", Area: "ops"})},
		model.Claim{Name: "cx-llm", Scope: cx}, model.Claim{Name: "ops-llm", Scope: domain.Scope{Company: "acme", Area: "ops"}})

	for _, name := range []string{"cx-llm", "nobody"} {
		_, err := r.Planner(name, finance, model.Config{Model: "m"}, nil)
		if err == nil {
			t.Fatalf("Planner(%q) from acme/finance found something", name)
		}
		if strings.Contains(err.Error(), "available") || strings.Contains(err.Error(), "ops-llm") ||
			(name != "cx-llm" && strings.Contains(err.Error(), "cx-llm")) {
			t.Errorf("Planner(%q) refusal = %q, names another provider", name, err)
		}
	}
}

/*
Configured holds the scope of a name the administration claims and this process
could not build, switched off or sealed: a plain set of names would answer yes
to every scope for another area's disabled provider.
*/
func TestConfigured_aClaimedNameItCouldNotBuild_keepsItsScope(t *testing.T) {
	t.Parallel()

	r := model.NewRegistry(nil)
	r.SetConfigured(nil, model.Claim{Name: "sealed", Scope: cx})

	if !r.Configured("sealed", cx) {
		t.Error("Configured(sealed, acme/cx) = false, want the claim inside its scope")
	}
	if r.Configured("sealed", finance) {
		t.Error("Configured(sealed, acme/finance) = true, want nothing outside its scope")
	}
}

func TestUsable_isJudgedAtTheScopeAsked(t *testing.T) {
	t.Parallel()

	r := model.NewRegistry(nil)
	r.SetConfigured([]model.Provider{areaProvider("cx-llm", cx)}, model.Claim{Name: "cx-llm", Scope: cx})
	if !r.Usable("cx-llm", cx) || r.Usable("cx-llm", finance) {
		t.Errorf("Usable = %v at acme/cx, %v at acme/finance; want true, false",
			r.Usable("cx-llm", cx), r.Usable("cx-llm", finance))
	}
}

// A planner built for one scope must not outlive a move of its provider to
// another: the revision moves when only the scope does.
func TestRevision_movesWhenOnlyTheScopeChanges(t *testing.T) {
	t.Parallel()

	r := model.NewRegistry(nil)
	r.SetConfigured([]model.Provider{areaProvider("llm", installation)}, model.Claim{Name: "llm", Scope: installation})
	before := r.Revision()
	r.SetConfigured([]model.Provider{areaProvider("llm", cx)}, model.Claim{Name: "llm", Scope: cx})
	if r.Revision() == before {
		t.Fatal("the revision stayed when the provider moved from the installation to acme/cx")
	}
}

// The environment's providers are the installation's when they name no scope.
func TestRegister_aProviderNamingNoScope_isTheInstallations(t *testing.T) {
	t.Parallel()

	r := model.NewRegistry(nil)
	if err := r.Register(model.Provider{Name: "from-env", Kind: model.KindOpenAICompatible, BaseURL: "http://127.0.0.1:1"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !r.Usable("from-env", cx) || !r.Configured("from-env", finance) {
		t.Error("an environment provider is not every scope's")
	}
}

// The authoring assistant and the token count run as the platform, not as a
// run of an area: they reach the installation's providers alone, so an area's
// key never pays for the platform's work.
func TestCompleterAndCounter_reachOnlyTheInstallationsProviders(t *testing.T) {
	t.Parallel()

	r := model.NewRegistry(nil)
	r.SetConfigured([]model.Provider{
		{Name: "shared", Kind: model.KindAnthropic, Scope: installation},
		{Name: "cx-claude", Kind: model.KindAnthropic, Scope: cx},
	}, model.Claim{Name: "shared", Scope: installation}, model.Claim{Name: "cx-claude", Scope: cx})

	if _, err := r.Completer("shared", model.Config{Model: "m"}); err != nil {
		t.Errorf("Completer(shared): %v", err)
	}
	if _, err := r.Counter("shared", model.Config{Model: "m"}); err != nil {
		t.Errorf("Counter(shared): %v", err)
	}
	if _, err := r.Completer("cx-claude", model.Config{Model: "m"}); err == nil {
		t.Error("the platform's assistant was built on acme/cx's provider")
	}
	if _, err := r.Counter("cx-claude", model.Config{Model: "m"}); err == nil {
		t.Error("a token count was built on acme/cx's provider")
	}
}
