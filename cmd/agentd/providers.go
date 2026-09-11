// Command agentd is the FuseOne Agents server.
//
// One binary, one Postgres, nothing else required (PRD DE-01). Subcommands
// select the role a process plays inside the installation.
package main

// Model providers: where they come from, and how an edit reaches a process
// that is already running.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/admin"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
)

// Model providers: where they come from, and how an edit reaches a process
// that is already running.

/*
providerConfig is the administration area, as this wiring needs it.

Declared here rather than taken as the concrete type because what happens when
one provider cannot be read is the whole point of the code below, and a test
that cannot make a credential refuse to open cannot say anything about it.
*/
type providerConfig interface {
	Providers(ctx context.Context) ([]domain.ModelProvider, error)
	Credential(ctx context.Context, name string) (string, error)
	Prices(ctx context.Context) ([]admin.ModelPrice, error)
}

// registerConfigured takes providers from the administration area, credential
// and all. This is where a key leaves the vault, and the only place it does.
func registerConfigured(ctx context.Context, registry *model.Registry, integrations *admin.Integrations) error {
	if integrations == nil {
		registerFromEnv(registry)
		return nil
	}
	failed, err := applyConfiguration(ctx, registry, integrations)
	if err != nil {
		return err
	}
	for name, why := range failed {
		// Loud at boot, and the run still fails: this is the difference
		// between an installation that is misconfigured and one that is lying
		// about which endpoint it speaks to.
		slog.Error("provider configured but unusable; it answers as not configured",
			"provider", name, "err", why)
	}
	return nil
}

/*
configureFrom replaces the registry's administration-supplied providers.

A provider that cannot be built is skipped and named, and the rest are
registered. It used to return at the first failure, which in the serve process
— where the vault is optional — meant one sealed credential erased every
configured provider at once. What the operator saw was not an error: the
environment's own provider was still there under the same name, so a run
reached a different endpoint with a different credential and the only evidence
was a vendor answering that the model does not exist.

A name that is configured and could not be built is claimed all the same, and
claimed empty: the environment must not quietly fill it. "Provider not
configured" is a sentence somebody can act on; a request to the wrong host is
not.
*/
func configureFrom(
	ctx context.Context, registry *model.Registry,
	from providerConfig, priced map[string]map[string]model.Prices,
) (map[string]string, error) {
	configured, err := from.Providers(ctx)
	if err != nil {
		return nil, fmt.Errorf("read configured providers: %w", err)
	}

	failed := map[string]string{}
	ready := make([]model.Provider, 0, len(configured))
	claimed := make([]string, 0, len(configured))
	for _, p := range configured {
		if !p.Enabled {
			continue
		}
		claimed = append(claimed, p.Name)
		provider, err := providerFrom(ctx, from, p, priced[p.Name])
		if err != nil {
			// Reported by the caller rather than here: at boot every failure
			// is news, and on a refresh only a change is. Logging it in the
			// loop turned a process serving without a master key into two
			// lines a minute, for ever, about a state somebody had chosen.
			failed[p.Name] = err.Error()
			continue
		}
		ready = append(ready, provider)
		slog.Info("provider configured", "provider", p.Name,
			"source", "administration", "priced_models", len(provider.Prices))
	}
	registry.SetConfigured(ready, claimed...)
	return failed, nil
}

/*
applyConfiguration is one pass of "what this installation is configured with".

Administration first and the environment after it, which is the precedence that
already held at boot: configuration somebody can audit outranks configuration
nobody can see. Prices last, because they apply to both.

Run again on a timer, so an address, a protocol or a credential edited in the
console reaches a process that is already running — which is what the price
refresh has always done for rates, and what everything else was missing.
*/
func applyConfiguration(
	ctx context.Context, registry *model.Registry, from providerConfig,
) (map[string]string, error) {
	// The environment is the layer underneath, and it is applied however this
	// pass ends: a read that failed leaves the last good configuration in
	// place, and the names nothing claims are still the environment's to fill.
	defer registerFromEnv(registry)

	priced, err := pricesFrom(ctx, from)
	if err != nil {
		return nil, err
	}
	failed, err := configureFrom(ctx, registry, from, priced)
	if err != nil {
		return nil, err
	}
	registry.SetPrices(priced)
	return failed, nil
}

// providerFrom builds one provider, preset quirks and credential included.
func providerFrom(
	ctx context.Context, from providerConfig,
	p domain.ModelProvider, priced map[string]model.Prices,
) (model.Provider, error) {
	provider := model.Provider{
		Name: p.Name, Kind: model.Kind(p.Kind), BaseURL: p.BaseURL,
	}
	// A preset fills in the quirks — which optional fields the endpoint
	// tolerates, whether it reports cached tokens — that a base URL alone
	// cannot express.
	if preset, ok := model.Preset(p.Name); ok {
		preset.BaseURL, preset.Kind = p.BaseURL, provider.Kind
		provider = preset
	}
	if p.HasKey {
		key, err := from.Credential(ctx, p.Name)
		if err != nil {
			return model.Provider{}, fmt.Errorf("open credential: %w", err)
		}
		provider.APIKey = key
	}
	provider.Prices = priced
	if provider.Kind == model.KindOpenAICompatible && provider.BaseURL == "" {
		return model.Provider{}, fmt.Errorf("an OpenAI-compatible provider needs an address")
	}
	return provider, nil
}

// pricesFrom reads the installation's own rates.
//
// They are live configuration, unlike a published agent version. Processes
// refresh them on a timer so saving a rate does not require a deploy, while
// model calls still run without a database read on every turn.
func pricesFrom(ctx context.Context, from providerConfig) (map[string]map[string]model.Prices, error) {
	priced := map[string]map[string]model.Prices{}
	rates, err := from.Prices(ctx)
	if err != nil {
		return nil, fmt.Errorf("read configured prices: %w", err)
	}
	for _, r := range rates {
		if priced[r.Provider] == nil {
			priced[r.Provider] = map[string]model.Prices{}
		}
		priced[r.Provider][r.Model] = model.Prices{
			InputMicros:      r.InputMicros,
			OutputMicros:     r.OutputMicros,
			CacheReadMicros:  r.CacheReadMicros,
			CacheWriteMicros: r.CacheWriteMicros,
		}
	}
	return priced, nil
}

// configRefresh bounds how long an edit in the console takes to reach a running
// process — a rate, and equally an address, a protocol or a credential. A
// failed read leaves the last good configuration in place: a refresh must not
// turn a working provider into a missing one, or a priced model back into zero.
const configRefresh = 30 * time.Second

/*
watchConfiguration keeps a running process agreeing with the console.

Only prices used to refresh. Everything else about a provider was read once at
boot, so pointing one at a proxy changed nothing until somebody restarted the
process — and the failure was silent, because the endpoint the process had
started with went on answering, authenticated, until the vendor reported a model
it had never heard of.

Failures are logged on change rather than every pass: at boot each one is news,
and after that only a provider that has just started or stopped working is.
*/
func watchConfiguration(ctx context.Context, registry *model.Registry, integrations *admin.Integrations) {
	if integrations == nil {
		return
	}
	ticker := time.NewTicker(configRefresh)
	defer ticker.Stop()

	reported := map[string]string{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			failed, err := applyConfiguration(ctx, registry, integrations)
			if err != nil && ctx.Err() == nil {
				slog.Warn("could not refresh the model configuration", "err", err)
				continue
			}
			for name, why := range failed {
				if reported[name] != why {
					slog.Error("provider configured but unusable; it answers as not configured",
						"provider", name, "err", why)
				}
			}
			for name := range reported {
				if _, still := failed[name]; !still {
					slog.Info("provider is usable again", "provider", name)
				}
			}
			reported = failed
		}
	}
}

// registerFromEnv keeps the environment working for an installation that has
// no administrator yet, and for local development. A provider already
// configured in the administration area wins: configuration somebody can audit
// outranks configuration nobody can see.
func registerFromEnv(registry *model.Registry) {
	existing := make(map[string]struct{}, len(registry.Names()))
	for _, name := range registry.Names() {
		existing[name] = struct{}{}
	}

	for _, name := range model.PresetNames() {
		key := os.Getenv(envKeyFor(name))
		if key == "" {
			continue
		}
		if _, taken := existing[name]; taken {
			slog.Info("ignoring environment credential; the administration area configures this provider",
				"provider", name)
			continue
		}
		p, _ := model.Preset(name)
		p.APIKey = key
		if base := os.Getenv(envBaseFor(name)); base != "" {
			p.BaseURL = base
		}
		if err := registry.Register(p); err != nil {
			slog.Warn("could not register provider from environment", "provider", name, "err", err)
			continue
		}
		slog.Info("provider configured", "provider", name, "source", "environment")
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
