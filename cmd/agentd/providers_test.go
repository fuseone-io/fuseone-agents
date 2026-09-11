package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
)

/*
A credential that cannot be opened does not hand the name to the environment.

The serve process holds the vault only if a master key was given to it, so
opening a sealed credential there can fail — and this returned at the first
failure, which meant one unreadable provider erased every configured provider
at once. What the operator saw was not an error. The environment had registered
a provider under the same name at boot, so the run reached a different endpoint
with a different credential, authenticated, and the only evidence was the
vendor answering that the model does not exist.

The name is claimed and held empty instead. "Provider not configured" is a
sentence somebody can act on.
*/
func TestConfigureFrom_aCredentialThatCannotBeOpened_doesNotFallBackToAnotherEndpoint(t *testing.T) {
	t.Parallel()
	registry := model.NewRegistry(nil)

	// What the environment put there at boot.
	if err := registry.Register(model.Provider{
		Name: "anthropic", Kind: model.KindAnthropic, APIKey: "from-the-environment",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := configureFrom(t.Context(), registry, &stubConfig{
		providers: []domain.ModelProvider{
			{Name: "anthropic", Kind: "anthropic", BaseURL: "https://litellm.internal", Enabled: true, HasKey: true},
			{Name: "vllm", Kind: "openai_compatible", BaseURL: "http://vllm.internal/v1", Enabled: true},
		},
		sealed: map[string]bool{"anthropic": true},
	}, nil)
	if err != nil {
		t.Fatalf("configureFrom: %v", err)
	}

	if _, err := registry.Planner("anthropic", model.Config{Model: "anthropic-claude-sonnet-5"}, nil); err == nil {
		t.Error("a provider whose credential could not be opened answered as configured")
	}
	// And the one beside it is registered: a failure is about one provider.
	if names := registry.Names(); !slices.Equal(names, []string{"vllm"}) {
		t.Errorf("names = %v, want the provider that could be built", names)
	}
}

// An OpenAI-compatible provider with no address is the same kind of lie: the
// client has no endpoint to fall back to, and registering it would produce a
// request to nowhere at the first turn rather than a refusal now.
func TestConfigureFrom_anOpenAICompatibleProviderWithNoAddress_isNotRegistered(t *testing.T) {
	t.Parallel()
	registry := model.NewRegistry(nil)

	if _, err := configureFrom(t.Context(), registry, &stubConfig{
		providers: []domain.ModelProvider{
			{Name: "litellm", Kind: "openai_compatible", Enabled: true},
		},
	}, nil); err != nil {
		t.Fatalf("configureFrom: %v", err)
	}
	if names := registry.Names(); len(names) != 0 {
		t.Errorf("names = %v, want nothing registered", names)
	}
}

// stubConfig is the administration area, with a vault that refuses some of it.
//
// A sealed provider answers the way the real read does when the master key is
// missing: the row says a credential is stored and none came back.
type stubConfig struct {
	providers    []domain.ModelProvider
	sealed       map[string]bool
	rates        []admin.ModelPrice
	providersErr error
}

func (s *stubConfig) ProvidersWithCredentials(context.Context) ([]admin.ConfiguredProvider, error) {
	if s.providersErr != nil {
		return nil, s.providersErr
	}
	out := make([]admin.ConfiguredProvider, 0, len(s.providers))
	for _, p := range s.providers {
		one := admin.ConfiguredProvider{ModelProvider: p}
		switch {
		case !p.HasKey:
		case s.sealed[p.Name]:
			one.Unreadable = "no vault: the master key was not given to this process"
		default:
			one.APIKey = "opened-" + p.Name
		}
		out = append(out, one)
	}
	return out, nil
}

func (s *stubConfig) Prices(context.Context) ([]admin.ModelPrice, error) { return s.rates, nil }

/*
An address changed in the console reaches a process that is already running.

Providers were bound once, at boot, in both processes — only prices refreshed.
So an operator who pointed a provider at their own proxy watched runs go on
speaking to the endpoint the process had started with, authenticated, until
somebody restarted it. There was no error to read: the vendor answered that the
model does not exist, which is true, and says nothing about why it was asked.
*/
func TestApplyConfiguration_anAddressChangedWhileRunning_isWhereTheNextRequestGoes(t *testing.T) {
	vendor, proxy := answering(t), answering(t)
	registry := model.NewRegistry(proxy.Client())

	config := &stubConfig{providers: []domain.ModelProvider{
		{Name: "anthropic", Kind: "anthropic", BaseURL: vendor.URL, Enabled: true, HasKey: true},
	}}
	if _, err := applyConfiguration(t.Context(), registry, config); err != nil {
		t.Fatalf("applyConfiguration: %v", err)
	}

	// The operator points it at their proxy, with the process still running.
	config.providers[0].BaseURL = proxy.URL
	if _, err := applyConfiguration(t.Context(), registry, config); err != nil {
		t.Fatalf("applyConfiguration again: %v", err)
	}

	counter, err := registry.Counter("anthropic", model.Config{Model: "anthropic-claude-sonnet-5"})
	if err != nil {
		t.Fatalf("Counter: %v", err)
	}
	if _, err := counter.Count(t.Context(), "hello"); err != nil {
		t.Fatalf("Count: %v", err)
	}
	if vendor.hits() != 0 {
		t.Error("the request went to the address the process started with")
	}
	if proxy.hits() != 1 {
		t.Errorf("the proxy was asked %d times, want once", proxy.hits())
	}
}

// And a provider removed from the console stops answering, rather than living
// on in the process for as long as it runs.
func TestApplyConfiguration_aProviderRemovedWhileRunning_stopsAnswering(t *testing.T) {
	endpoint := answering(t)
	registry := model.NewRegistry(endpoint.Client())

	config := &stubConfig{providers: []domain.ModelProvider{
		{Name: "litellm", Kind: "anthropic", BaseURL: endpoint.URL, Enabled: true, HasKey: true},
	}}
	if _, err := applyConfiguration(t.Context(), registry, config); err != nil {
		t.Fatalf("applyConfiguration: %v", err)
	}
	config.providers = nil
	if _, err := applyConfiguration(t.Context(), registry, config); err != nil {
		t.Fatalf("applyConfiguration again: %v", err)
	}

	if _, err := registry.Planner("litellm", model.Config{Model: "x"}, nil); err == nil {
		t.Error("a provider nobody configures any more still answers")
	}
}

// answering is an endpoint that counts what it was asked.
type answers struct {
	*httptest.Server
	mu    sync.Mutex
	asked int
}

func answering(t *testing.T) *answers {
	t.Helper()
	a := &answers{}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		a.mu.Lock()
		a.asked++
		a.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input_tokens":3}`))
	}))
	t.Cleanup(a.Close)
	return a
}

func (a *answers) hits() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.asked
}

/*
A name the console claims is not filled by the environment.

The claim was made in the registry and then handed straight back: the
environment fallback asked which providers were registered, and a name held
empty is not registered — so it looked free, and the variable filled it. The
substitution this whole path exists to prevent, reintroduced two lines later.

A provider somebody switched off is the same claim. Disabled means this
installation said no, not "use whatever the environment has under that name".
*/
func TestApplyConfiguration_aNameTheConsoleClaims_isNotFilledByTheEnvironment(t *testing.T) {
	for _, one := range []struct {
		name      string
		configure domain.ModelProvider
		sealed    bool
	}{
		{
			name: "configured, and its credential cannot be opened",
			configure: domain.ModelProvider{
				Name: "anthropic", Kind: "anthropic",
				BaseURL: "https://litellm.internal", Enabled: true, HasKey: true,
			},
			sealed: true,
		},
		{
			name: "configured and switched off",
			configure: domain.ModelProvider{
				Name: "anthropic", Kind: "anthropic",
				BaseURL: "https://litellm.internal", Enabled: false,
			},
		},
	} {
		t.Run(one.name, func(t *testing.T) {
			t.Setenv("ANTHROPIC_API_KEY", "the-vendors-key")
			registry := model.NewRegistry(nil)

			config := &stubConfig{providers: []domain.ModelProvider{one.configure}}
			if one.sealed {
				config.sealed = map[string]bool{"anthropic": true}
			}
			if _, err := applyConfiguration(t.Context(), registry, config); err != nil {
				t.Fatalf("applyConfiguration: %v", err)
			}

			if slices.Contains(registry.Names(), "anthropic") {
				t.Error("the environment filled a name the console had claimed")
			}
		})
	}
}

// And a name nobody configures is still the environment's to fill: that is the
// installation with no administrator yet, and local development.
func TestApplyConfiguration_aNameNobodyConfigures_isStillTheEnvironments(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "the-vendors-key")
	registry := model.NewRegistry(nil)

	if _, err := applyConfiguration(t.Context(), registry, &stubConfig{}); err != nil {
		t.Fatalf("applyConfiguration: %v", err)
	}
	if !slices.Contains(registry.Names(), "anthropic") {
		t.Error("the environment's provider was dropped")
	}
}

/*
A provider the environment supplies starts life with its configured rate.

The environment layer ran in a defer, so it landed after the rates were
applied — and a provider it registered began with no rate at all until the next
pass, thirty seconds later. Runs opened in that window record tokens with no
money against them, and a ceiling stated in money has nothing to measure.
*/
func TestApplyConfiguration_aProviderFromTheEnvironment_hasItsRateOnTheFirstPass(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "from-the-environment")
	registry := model.NewRegistry(nil)

	if _, err := applyConfiguration(t.Context(), registry, &stubConfig{
		rates: []admin.ModelPrice{{
			Provider: "anthropic", Model: "claude-opus-5", InputMicros: 5, OutputMicros: 25,
		}},
	}); err != nil {
		t.Fatalf("applyConfiguration: %v", err)
	}

	price, priced, err := registry.PriceFor("anthropic", "claude-opus-5")
	if err != nil {
		t.Fatalf("PriceFor: %v", err)
	}
	if !priced || price.InputMicros != 5 {
		t.Errorf("rate = %+v priced=%v, want the configured rate on the first pass", price, priced)
	}
}

/*
A pass that changes nothing leaves every planner where it is.

The revision is what tells a resolver to rebuild the planner it cached for a
published version. Production hands every usable provider to SetConfigured twice
— once as a claim and once as a value — and the claim deleted the name before
the comparison could see it, so nothing ever looked unchanged. Every active
version rebuilt its planner every thirty seconds.
*/
func TestApplyConfiguration_twoIdenticalPasses_leaveTheRevision(t *testing.T) {
	registry := model.NewRegistry(nil)
	config := &stubConfig{providers: []domain.ModelProvider{
		{Name: "litellm", Kind: "openai_compatible", BaseURL: "https://litellm.internal/v1",
			Models: []string{"gemini/gemini-2.5-pro"}, Enabled: true, HasKey: true},
	}, rates: []admin.ModelPrice{{
		Provider: "litellm", Model: "gemini/gemini-2.5-pro", InputMicros: 3,
	}}}

	if _, err := applyConfiguration(t.Context(), registry, config); err != nil {
		t.Fatalf("applyConfiguration: %v", err)
	}
	settled := registry.Revision()
	for range 3 {
		if _, err := applyConfiguration(t.Context(), registry, config); err != nil {
			t.Fatalf("applyConfiguration again: %v", err)
		}
	}
	if registry.Revision() != settled {
		t.Errorf("revision moved to %d over three identical passes, from %d",
			registry.Revision(), settled)
	}
}

/*
Even a pass that could not read the providers leaves the environment priced.

The rates had been read, and the exit that gave up on the providers returned
before applying them — so on a fresh process whose first pass met a database
error, the environment's provider existed with no rate at all, and stayed that
way while the error lasted.
*/
func TestApplyConfiguration_whenTheProvidersCannotBeRead_theEnvironmentIsStillPriced(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "from-the-environment")
	registry := model.NewRegistry(nil)

	_, err := applyConfiguration(t.Context(), registry, &stubConfig{
		providersErr: errors.New("the database is away"),
		rates: []admin.ModelPrice{{
			Provider: "anthropic", Model: "claude-opus-5", InputMicros: 5,
		}},
	})
	if err == nil {
		t.Fatal("applyConfiguration: want the read failure reported")
	}

	price, priced, err := registry.PriceFor("anthropic", "claude-opus-5")
	if err != nil {
		t.Fatalf("PriceFor: %v", err)
	}
	if !priced || price.InputMicros != 5 {
		t.Errorf("rate = %+v priced=%v, want the environment's provider priced anyway", price, priced)
	}
}
