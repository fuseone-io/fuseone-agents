package model_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
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

/*
Administration replaces what it configured, and only that.

A provider was registered once at boot and never again, so an address edited in
the console reached nothing until the process restarted: the run went on
speaking to the endpoint the process happened to start with, authenticated,
with a credential nobody had chosen. It surfaced as a 404 from a vendor the
operator had already stopped pointing at.

Proved by where the request lands rather than by reading the registry back — a
provider that holds the right address and calls the wrong one is the bug this
exists to catch.
*/
func TestSetConfigured_anAddressEditedInTheConsole_isWhereTheRequestGoes(t *testing.T) {
	t.Parallel()

	vendor := recordingProvider(t)
	proxy := recordingProvider(t)

	registry := model.NewRegistry(proxy.Client())
	// What the environment put there at boot: the vendor's own endpoint.
	if err := registry.Register(model.Provider{
		Name: "anthropic", Kind: model.KindAnthropic,
		BaseURL: vendor.URL, APIKey: "from-the-environment",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	registry.SetConfigured([]model.Provider{{
		Name: "anthropic", Kind: model.KindAnthropic,
		BaseURL: proxy.URL, APIKey: "from-the-console",
	}})

	counter, err := registry.Counter("anthropic", model.Config{Model: "anthropic-claude-sonnet-5"})
	if err != nil {
		t.Fatalf("Counter: %v", err)
	}
	if _, err := counter.Count(t.Context(), "hello"); err != nil {
		t.Fatalf("Count: %v", err)
	}

	if vendor.hits() != 0 {
		t.Error("the request went to the endpoint the process started with")
	}
	if proxy.hits() != 1 {
		t.Fatalf("the proxy was asked %d times, want once", proxy.hits())
	}
	if key := proxy.key(); key != "from-the-console" {
		t.Errorf("credential = %q, want the one the console stored", key)
	}
}

// And a provider nobody configures any more is gone, without touching the one
// the environment supplied beside it. Register alone cannot express a removal,
// so a provider deleted in the console stayed alive in the worker for as long
// as it ran.
func TestSetConfigured_aProviderRemovedFromTheConsole_leavesTheRegistry(t *testing.T) {
	t.Parallel()
	registry := model.NewRegistry(nil)

	if err := registry.Register(model.Provider{
		Name: "ollama", Kind: model.KindOpenAICompatible, BaseURL: "http://127.0.0.1:11434/v1",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	registry.SetConfigured([]model.Provider{{
		Name: "litellm", Kind: model.KindOpenAICompatible, BaseURL: "https://litellm.internal/v1",
	}})
	registry.SetConfigured(nil)

	if names := registry.Names(); !slices.Equal(names, []string{"ollama"}) {
		t.Errorf("names = %v, want only the one the environment supplied", names)
	}
}

// recordingProvider is an endpoint that answers a token count and remembers
// that it was asked, and with which credential.
type recorded struct {
	*httptest.Server
	mu     sync.Mutex
	asked  int
	apiKey string
}

func recordingProvider(t *testing.T) *recorded {
	t.Helper()
	r := &recorded{}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.asked++
		r.apiKey = req.Header.Get("x-api-key")
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":3}`))
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *recorded) hits() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.asked
}

func (r *recorded) key() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.apiKey
}
