package model_test

import (
	"testing"

	"github.com/fuseone/agents/internal/model"
)

// Configured prices are the installation's; market defaults are public
// reference values in their own currency. They must not feed Cost.Micros,
// whose domain contract is the installation's currency.

func TestPlanner_fillsTheRateRegisteredForThatModel(t *testing.T) {
	t.Parallel()

	registry := model.NewRegistry(nil)
	if err := registry.Register(model.Provider{
		Name: "anthropic", Kind: model.KindAnthropic,
		Prices: map[string]model.Prices{
			"claude-opus-5": {InputMicros: 5_000_000, OutputMicros: 25_000_000},
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Filled by the registry rather than by each caller. A run and an
	// authoring call reach a provider through different paths, and a price
	// threaded through both is a price that ends up set in one of them.
	got, ok, err := registry.PriceFor("anthropic", "claude-opus-5")
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if !ok {
		t.Fatal("price was not reported as configured")
	}
	if got.InputMicros != 5_000_000 || got.OutputMicros != 25_000_000 {
		t.Errorf("got %+v", got)
	}
}

func TestPriceFor_aKnownModelNobodyPriced_isMissing(t *testing.T) {
	t.Parallel()

	registry := model.NewRegistry(nil)
	if err := registry.Register(model.Provider{Name: "anthropic", Kind: model.KindAnthropic}); err != nil {
		t.Fatalf("register: %v", err)
	}

	got, ok, err := registry.PriceFor("anthropic", "claude-opus-5")
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if ok {
		t.Fatal("unconfigured price reported as configured")
	}
	if got != (model.Prices{}) {
		t.Errorf("got %+v, want no accounting rate without a configured installation price", got)
	}
}

func TestPriceFor_anUnknownModelNobodyPriced_isMissing(t *testing.T) {
	t.Parallel()

	registry := model.NewRegistry(nil)
	if err := registry.Register(model.Provider{Name: "anthropic", Kind: model.KindAnthropic}); err != nil {
		t.Fatalf("register: %v", err)
	}

	got, ok, err := registry.PriceFor("anthropic", "claude-from-next-week")
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if ok {
		t.Fatal("unknown model price reported as configured")
	}
	// A public default exists only for models named in the bundled table. For
	// a new or custom model, zero says "no price" rather than inventing money.
	if got != (model.Prices{}) {
		t.Errorf("got %+v, want no price for an unknown model", got)
	}
}

func TestPriceFor_aConfiguredRateOverridesTheMarketDefault(t *testing.T) {
	t.Parallel()

	registry := model.NewRegistry(nil)
	if err := registry.Register(model.Provider{
		Name: "anthropic", Kind: model.KindAnthropic,
		Prices: map[string]model.Prices{
			"claude-opus-5": {InputMicros: 1, OutputMicros: 2},
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	got, ok, err := registry.PriceFor("anthropic", "claude-opus-5")
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if !ok {
		t.Fatal("price was not reported as configured")
	}
	if got.InputMicros != 1 || got.OutputMicros != 2 {
		t.Errorf("got %+v, want the configured contract rate", got)
	}
}

func TestPriceFor_aConfiguredZeroRate_isStillConfigured(t *testing.T) {
	t.Parallel()

	registry := model.NewRegistry(nil)
	if err := registry.Register(model.Provider{
		Name: "vllm", Kind: model.KindOpenAICompatible, BaseURL: "https://x/v1",
		Prices: map[string]model.Prices{
			"llama": {},
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	got, ok, err := registry.PriceFor("vllm", "llama")
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if !ok {
		t.Fatal("configured zero rate was collapsed into missing")
	}
	if got != (model.Prices{}) {
		t.Errorf("got %+v, want deliberate zero", got)
	}
}

func TestSetPrices_updatesRegisteredProvidersAndAdvancesTheRevision(t *testing.T) {
	t.Parallel()

	registry := model.NewRegistry(nil)
	if err := registry.Register(model.Provider{
		Name: "anthropic", Kind: model.KindAnthropic,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	before := registry.PriceRevision()

	if changed := registry.SetPrices(map[string]map[string]model.Prices{
		"anthropic": {
			"claude-opus-5": {InputMicros: 7_000_000},
		},
	}); !changed {
		t.Fatal("SetPrices reported no change")
	}
	got, ok, err := registry.PriceFor("anthropic", "claude-opus-5")
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if !ok {
		t.Fatal("refreshed price was not reported as configured")
	}
	if got.InputMicros != 7_000_000 {
		t.Fatalf("price = %+v, want refreshed rate", got)
	}
	if registry.PriceRevision() != before+1 {
		t.Fatalf("revision = %d, want %d", registry.PriceRevision(), before+1)
	}

	if changed := registry.SetPrices(map[string]map[string]model.Prices{
		"anthropic": {
			"claude-opus-5": {InputMicros: 7_000_000},
		},
	}); changed {
		t.Fatal("same price table should not advance the revision")
	}
	if registry.PriceRevision() != before+1 {
		t.Fatalf("revision changed on an identical table")
	}
}

func TestPlanner_carriesTheRateWithoutTheCallerAskingForIt(t *testing.T) {
	t.Parallel()

	registry := model.NewRegistry(nil)
	if err := registry.Register(model.Provider{
		Name: "openai", Kind: model.KindOpenAICompatible, BaseURL: "https://x/v1",
		Prices: map[string]model.Prices{"gpt": {InputMicros: 3_000_000}},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	// The caller passes a Config with no rate in it, as both callers do today,
	// and the planner still accounts in money. Threading the price through
	// every call site is how one of them ends up recording zero.
	planner, err := registry.Planner("openai", model.Config{Model: "gpt"}, nil)
	if err != nil {
		t.Fatalf("planner: %v", err)
	}
	if got := model.RateOf(planner); got.InputMicros != 3_000_000 {
		t.Errorf("got %+v", got)
	}
}
