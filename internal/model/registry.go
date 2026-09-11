package model

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/fuseone/agents/internal/engine"
)

/*
What this installation is configured with, and what it hands a run.

Split from the shape of a provider beside it because the two change for
different reasons: a Provider gains a field when a vendor grows a quirk worth
describing, and this changes when the rules about precedence, replacement and
staleness do.
*/

// Registry holds the providers an installation has configured and builds a
// planner for an agent's model configuration.
//
// The engine only ever sees engine.Planner, so which vendor answers a run is
// an installation setting rather than an architectural commitment.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	http      *http.Client
	revision  uint64
	// configured is which names the administration area last supplied, so a
	// refresh can express a removal without touching what the environment put
	// there beside it.
	configured map[string]bool
}

func NewRegistry(hc *http.Client) *Registry {
	return &Registry{providers: make(map[string]Provider), http: hc}
}

// Register adds or replaces a provider.
func (r *Registry) Register(p Provider) error {
	if p.Name == "" {
		return fmt.Errorf("model: provider needs a name")
	}
	if p.Kind == KindOpenAICompatible && p.BaseURL == "" {
		return fmt.Errorf("model: provider %q needs a base URL", p.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	p.Prices = clonePriceMap(p.Prices)
	r.providers[p.Name] = p
	return nil
}

/*
SetConfigured replaces the providers the administration area supplies.

Register alone could only add, so the registry was a photograph of boot: an
address edited in the console reached no running process, and the run went on
speaking to whatever endpoint that process had started with — authenticated,
with a credential nobody had chosen, and reported by the vendor as a model that
does not exist. A provider deleted in the console survived just as long.

Replaced as a set, so a removal is expressible. Providers registered from the
environment are left alone unless the administration area now claims the same
name, which is the precedence that already held at boot: configuration somebody
can audit outranks configuration nobody can see.
*/
func (r *Registry) SetConfigured(ps []Provider, claimed ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	taken := make(map[string]bool, len(ps)+len(claimed))
	changed := false
	// Claimed first, so a name the administration area configured and this
	// process could not build is held empty rather than left to the
	// environment. Filled from somewhere else it is not a misconfiguration any
	// more, it is a request to an endpoint nobody chose.
	for _, name := range claimed {
		taken[name] = true
		if _, held := r.providers[name]; held {
			delete(r.providers, name)
			changed = true
		}
	}
	for _, p := range ps {
		p.Prices = clonePriceMap(p.Prices)
		p.Models = slices.Clone(p.Models)
		if before, held := r.providers[p.Name]; !held || !sameProvider(before, p) {
			changed = true
		}
		r.providers[p.Name] = p
		taken[p.Name] = true
	}
	for name := range r.configured {
		if !taken[name] {
			delete(r.providers, name)
			changed = true
		}
	}
	r.configured = taken
	if changed {
		r.revision++
	}
}

// sameProvider answers whether a planner built from one would be built the same
// from the other. Every field, because every field is something a request
// carries — an address, a protocol, a credential, a rate, and the names offered
// to an author.
func sameProvider(a, b Provider) bool {
	return a.Name == b.Name && a.Kind == b.Kind && a.BaseURL == b.BaseURL &&
		a.APIKey == b.APIKey &&
		a.SupportsReasoningEffort == b.SupportsReasoningEffort &&
		a.ReportsCachedTokens == b.ReportsCachedTokens &&
		slices.Equal(a.Models, b.Models) &&
		maps.Equal(a.Headers, b.Headers) &&
		maps.Equal(a.Prices, b.Prices)
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Sorted(maps.Keys(r.providers))
}

/*
Revision changes when anything a planner was built from changes.

A planner owns the address, the protocol, the credential and the rate it was
built with. Specs are immutable by version; all four of those are live
administration state, so a resolver must not reuse a planner past this number.

It counted rate changes alone, which was the whole of this state when prices
were the only thing that refreshed. An address edited in the console then
reached the registry and stopped there: every version that had already run kept
a planner pointed at the old endpoint for as long as the process lived.
*/
func (r *Registry) Revision() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.revision
}

/*
Claimed is every name the administration area configures, including the ones
this process could not build and the ones somebody switched off.

Asked by the environment fallback, which otherwise reads the registry's
contents and sees an unclaimed name where there is a deliberate silence — and
fills it, which is the substitution this whole path exists to prevent.
*/
func (r *Registry) Claimed() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Sorted(maps.Keys(r.configured))
}

// SetPrices replaces the configured rates on registered providers.
//
// Provider credentials and endpoints are still connection state; this only
// refreshes the money table operators edit while the worker is running.
func (r *Registry) SetPrices(priced map[string]map[string]Prices) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	changed := false
	for name, provider := range r.providers {
		next := clonePriceMap(priced[name])
		if maps.Equal(provider.Prices, next) {
			continue
		}
		provider.Prices = next
		r.providers[name] = provider
		changed = true
	}
	if changed {
		r.revision++
	}
	return changed
}

// Planner builds the planner an agent runs on.
func (r *Registry) Planner(providerName string, cfg Config, tools ToolSchemas) (engine.Planner, error) {
	r.mu.RLock()
	p, ok := r.providers[providerName]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("model: provider %q is not configured; available: %v", providerName, r.Names())
	}

	return r.plannerFrom(p, withPrice(p, cfg), tools)
}

/*
plannerFrom builds a planner from one reading of a provider.

Taken as a value rather than looked up again, because the callers that need to
know a provider's kind used to read it, release the lock and ask for a planner —
which read the registry a second time. A refresh landing between the two
returned a planner of the other shape, and the type assertion that followed was
the answer to a configuration edit being a panic in a worker.

The environment is refused explicitly. The Anthropic client reads
ANTHROPIC_API_KEY and ANTHROPIC_BASE_URL on its own when it is given neither, so
a proxy configured without a credential was handed the vendor's — the operator's
own key, sent to an endpoint they did not choose it for.
*/
func (r *Registry) plannerFrom(p Provider, cfg Config, tools ToolSchemas) (engine.Planner, error) {
	switch p.Kind {
	case KindAnthropic:
		opts := []option.RequestOption{option.WithoutEnvironmentDefaults()}
		if p.APIKey != "" {
			opts = append(opts, option.WithAPIKey(p.APIKey))
		}
		if p.BaseURL != "" {
			opts = append(opts, option.WithBaseURL(p.BaseURL))
		}
		if r.http != nil {
			opts = append(opts, option.WithHTTPClient(r.http))
		}
		return New(anthropic.NewClient(opts...), p.Name, cfg, tools), nil

	case KindOpenAICompatible:
		return NewOpenAICompatible(p, cfg, tools, r.http), nil

	default:
		return nil, fmt.Errorf("model: provider %q has unknown kind %q", p.Name, p.Kind)
	}
}

func clonePriceMap(in map[string]Prices) map[string]Prices {
	if len(in) == 0 {
		return nil
	}
	return maps.Clone(in)
}
