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

// The authoring assistant runs as the platform and reaches the installation's
// providers alone. The token count is asked at the agent's scope and reaches
// what a run there would (NT-019).
func TestCompleterAndCounter_reachWhatTheirScopeReaches(t *testing.T) {
	t.Parallel()

	r := model.NewRegistry(nil)
	r.SetConfigured([]model.Provider{
		{Name: "shared", Kind: model.KindAnthropic, Scope: installation},
		{Name: "cx-claude", Kind: model.KindAnthropic, Scope: cx},
	}, model.Claim{Name: "shared", Scope: installation}, model.Claim{Name: "cx-claude", Scope: cx})

	if _, err := r.Completer("shared", model.Config{Model: "m"}); err != nil {
		t.Errorf("Completer(shared): %v", err)
	}
	if _, err := r.Completer("cx-claude", model.Config{Model: "m"}); err == nil {
		t.Error("the platform's assistant was built on acme/cx's provider")
	}
	if _, err := r.Counter("cx-claude", cx, model.Config{Model: "m"}); err != nil {
		t.Errorf("Counter(cx-claude) at acme/cx: %v", err)
	}
	if _, err := r.Counter("shared", cx, model.Config{Model: "m"}); err != nil {
		t.Errorf("Counter(shared) at acme/cx: %v", err)
	}
	if _, err := r.Counter("cx-claude", finance, model.Config{Model: "m"}); err == nil {
		t.Error("a count at acme/finance was built on acme/cx's provider")
	}
	if _, err := r.Counter("cx-claude", installation, model.Config{Model: "m"}); err == nil {
		t.Error("a count with no agent scope was built on acme/cx's provider")
	}
}

// A name the environment supplies stays the environment's while the console
// overrides it: the override is the installation's, and once removed the
// environment's answers again (NT-019). A name it never supplied is not.
func TestFromEnvironment_isWhatTheEnvironmentSupplies(t *testing.T) {
	t.Parallel()
	r := model.NewRegistry(nil)
	_ = r.Register(model.Provider{Name: "from-env", Kind: model.KindAnthropic})
	_ = r.Register(model.Provider{Name: "overridden", Kind: model.KindAnthropic})
	r.SetConfigured([]model.Provider{{Name: "console-only", Kind: model.KindAnthropic, Scope: installation}},
		model.Claim{Name: "overridden", Scope: installation}, model.Claim{Name: "console-only", Scope: installation})
	if !r.FromEnvironment("from-env") || !r.FromEnvironment("overridden") || r.FromEnvironment("console-only") {
		t.Errorf("FromEnvironment = %v, %v, %v; want true, true, false",
			r.FromEnvironment("from-env"), r.FromEnvironment("overridden"), r.FromEnvironment("console-only"))
	}
}

// The environment's provider is remembered as the environment's: a console
// override replaces it while it lasts, and once removed the environment's
// answers again under the name, and still holds the name against an area.
func TestFromEnvironment_survivesAnOverrideThatWasRemoved(t *testing.T) {
	t.Parallel()
	r := model.NewRegistry(nil)
	if err := r.Register(model.Provider{Name: "anthropic", Kind: model.KindAnthropic, APIKey: "env-key"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	r.SetConfigured([]model.Provider{{Name: "anthropic", Kind: model.KindAnthropic, APIKey: "console-key", Scope: installation}},
		model.Claim{Name: "anthropic", Scope: installation})
	r.SetConfigured(nil)

	if !r.FromEnvironment("anthropic") {
		t.Error("FromEnvironment(anthropic) = false after the override was removed")
	}
	if !r.Usable("anthropic", cx) {
		t.Error("the environment's provider did not answer again once the override was removed")
	}
}

// A name the console claims and could not build stays empty: the environment
// does not fill it, removed override or not.
func TestSetConfigured_aClaimNotBuilt_isNotFilledByTheEnvironment(t *testing.T) {
	t.Parallel()
	r := model.NewRegistry(nil)
	_ = r.Register(model.Provider{Name: "anthropic", Kind: model.KindAnthropic})
	r.SetConfigured(nil, model.Claim{Name: "anthropic", Scope: installation})
	if r.Usable("anthropic", installation) {
		t.Error("a claimed name not built was filled by the environment")
	}
}
