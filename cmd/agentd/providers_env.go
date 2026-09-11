package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/fuseone/agents/internal/model"
)

// The layer underneath the administration area: providers an installation
// names in its environment, and the variables that name them.

// registerFromEnv keeps the environment working for an installation that has
// no administrator yet, and for local development. A provider already
// configured in the administration area wins: configuration somebody can audit
// outranks configuration nobody can see.
func registerFromEnv(registry *model.Registry, priced map[string]map[string]model.Prices) {
	existing := make(map[string]struct{}, len(registry.Names()))
	for _, name := range registry.Names() {
		existing[name] = struct{}{}
	}
	// And the names the administration area claims, which are not the same as
	// the names it could fill. A provider whose credential would not open, or
	// one somebody switched off, holds its name and registers nothing — read
	// from the registry's contents alone that name looks free, and filling it
	// is the substitution this path exists to prevent.
	for _, name := range registry.Claimed() {
		existing[name] = struct{}{}
	}

	for _, name := range model.PresetNames() {
		key := os.Getenv(envKeyFor(name))
		if key == "" {
			continue
		}
		if _, taken := existing[name]; taken {
			// Debug, because this is a steady state somebody chose and this
			// runs every thirty seconds. Said at Info it is two lines a minute
			// about a decision that is not changing.
			slog.Debug("ignoring environment credential; the administration area configures this provider",
				"provider", name)
			continue
		}
		p, _ := model.Preset(name)
		p.APIKey = key
		// Priced as it is registered, not afterwards. Applied as a second step
		// it left a window — and on the pass where reading the providers
		// failed, no second step ran at all: the provider existed with no rate
		// until some later refresh, recording tokens with no money against
		// them and leaving a ceiling stated in money with nothing to measure.
		p.Prices = priced[name]
		if base := os.Getenv(envBaseFor(name)); base != "" {
			p.BaseURL = base
		}
		if err := registry.Register(p); err != nil {
			slog.Warn("could not register provider from environment", "provider", name, "err", err)
			continue
		}
		slog.Info("provider configured from the environment", "provider", name)
	}
}

// envKeyFor names the variable holding a provider's credential, e.g.
// ANTHROPIC_API_KEY, DEEPSEEK_API_KEY.
func envKeyFor(provider string) string {
	return strings.ToUpper(provider) + "_API_KEY"
}

// envBaseFor overrides a provider's endpoint — required for the self-hosted
// presets, which ship without one.
func envBaseFor(provider string) string {
	return strings.ToUpper(provider) + "_BASE_URL"
}
