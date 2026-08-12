package model_test

import (
	"testing"

	"github.com/fuseone/agents/internal/model"
)

// Prices are the installation's, never the platform's: they vary by contract,
// and a rate shipped in a binary would quietly misreport what a customer with
// a negotiated discount actually pays.

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
	got, err := registry.PriceFor("anthropic", "claude-opus-5")
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	if got.InputMicros != 5_000_000 || got.OutputMicros != 25_000_000 {
		t.Errorf("got %+v", got)
	}
}

func TestPriceFor_aModelNobodyPriced_isZeroRatherThanAGuess(t *testing.T) {
	t.Parallel()

	registry := model.NewRegistry(nil)
	if err := registry.Register(model.Provider{Name: "anthropic", Kind: model.KindAnthropic}); err != nil {
		t.Fatalf("register: %v", err)
	}

	got, err := registry.PriceFor("anthropic", "claude-opus-5")
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	// Zero means the ledger records tokens and no money, which is the honest
	// answer. A default rate would put a number an operator trusts next to a
	// figure nobody supplied.
	if got != (model.Prices{}) {
		t.Errorf("got %+v, want zero", got)
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
