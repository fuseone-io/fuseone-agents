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

// Kind is how a provider is spoken to, not who sells it.
//
// There are only two wire formats worth implementing: Anthropic's, and the
// chat-completions shape everyone else settled on. A new vendor is a row in
// the preset table, not a new client.
type Kind string

const (
	KindAnthropic        Kind = "anthropic"
	KindOpenAICompatible Kind = "openai_compatible"
)

// Provider is one configured endpoint.
type Provider struct {
	Name    string
	Kind    Kind
	BaseURL string

	// Models the platform knows this provider serves. Suggestions, never a
	// closed set: a list shipped in a binary ages between releases, and one
	// that blocked a model released last week would be worse than no list.
	// Filled only where it is known — guessing a name produces a 404 at the
	// first run rather than an error anybody can act on.
	Models []string

	// Prices are what this installation pays per model, in micros per million
	// tokens. A configured rate wins over the bundled market default because
	// contracts vary; absent falls back to a named public default when the
	// platform knows one. Unknown models still record tokens and no money.
	Prices map[string]Prices

	APIKey  string
	Headers map[string]string

	// SupportsReasoningEffort gates the reasoning_effort field. Providers that
	// reject unknown fields return a 400 when it is sent blindly, so it is
	// opt-in per provider rather than always-on.
	SupportsReasoningEffort bool

	// ReportsCachedTokens records whether usage accounting distinguishes cache
	// reads from ordinary input. Where it does not, cached tokens are billed
	// as input — an over-estimate, never a silent under-count. The console
	// surfaces this so nobody reads a rounded-up figure as measured.
	ReportsCachedTokens bool
}

// Presets are the providers an installation can enable without knowing a base
// URL. Credentials are never here — they come from the vault at registration.
//
// Anthropic is the reference implementation: it is the only one that exposes
// explicit cache breakpoints, an effort control, and separate cache read and
// write accounting, so cost figures are exact there and approximate elsewhere.
var Presets = map[string]Provider{
	"anthropic": {
		Name: "anthropic", Kind: KindAnthropic,
		SupportsReasoningEffort: true, ReportsCachedTokens: true,
		Models: []string{"claude-opus-5", "claude-sonnet-5", "claude-haiku-4-5"},
	},
	"openai": {
		Name: "openai", Kind: KindOpenAICompatible,
		BaseURL:                 "https://api.openai.com/v1",
		SupportsReasoningEffort: true, ReportsCachedTokens: true,
	},
	"deepseek": {
		Name: "deepseek", Kind: KindOpenAICompatible,
		BaseURL: "https://api.deepseek.com/v1",
		// Reports cache hits under its own field name, handled in cost().
		ReportsCachedTokens: true,
	},
	"kimi": {
		Name: "kimi", Kind: KindOpenAICompatible,
		BaseURL: "https://api.moonshot.cn/v1",
	},
	"groq": {
		Name: "groq", Kind: KindOpenAICompatible,
		BaseURL: "https://api.groq.com/openai/v1",
	},
	"xai": {
		Name: "xai", Kind: KindOpenAICompatible,
		BaseURL: "https://api.x.ai/v1",
	},
	"mistral": {
		Name: "mistral", Kind: KindOpenAICompatible,
		BaseURL: "https://api.mistral.ai/v1",
	},
	"together": {
		Name: "together", Kind: KindOpenAICompatible,
		BaseURL: "https://api.together.xyz/v1",
	},
	// Self-hosted. The base URL is the installation's own, so the preset only
	// carries the shape — this is the path for a customer that cannot let
	// prompts leave its network at all.
	"vllm": {
		Name: "vllm", Kind: KindOpenAICompatible,
	},
	"ollama": {
		Name: "ollama", Kind: KindOpenAICompatible,
		BaseURL: "http://127.0.0.1:11434/v1",
	},
}

// Preset returns a copy of a known provider's shape.
func Preset(name string) (Provider, bool) {
	p, ok := Presets[name]
	if !ok {
		return Provider{}, false
	}
	p.Headers = maps.Clone(p.Headers)
	return p, true
}

// PresetNames lists the known providers, sorted.
func PresetNames() []string {
	return slices.Sorted(maps.Keys(Presets))
}

// Registry holds the providers an installation has configured and builds a
// planner for an agent's model configuration.
//
// The engine only ever sees engine.Planner, so which vendor answers a run is
// an installation setting rather than an architectural commitment.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	http      *http.Client
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
	r.providers[p.Name] = p
	return nil
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Sorted(maps.Keys(r.providers))
}

// Planner builds the planner an agent runs on.
func (r *Registry) Planner(providerName string, cfg Config, tools ToolSchemas) (engine.Planner, error) {
	cfg = r.withPrice(providerName, cfg)
	r.mu.RLock()
	p, ok := r.providers[providerName]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("model: provider %q is not configured; available: %v", providerName, r.Names())
	}

	switch p.Kind {
	case KindAnthropic:
		opts := []option.RequestOption{}
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
