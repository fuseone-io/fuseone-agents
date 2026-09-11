// Command agentd is the FuseOne Agents server.
//
// One binary, one Postgres, nothing else required (PRD DE-01). Subcommands
// select the role a process plays inside the installation.
package main

// Model providers: where they come from, and how an edit reaches a process
// that is already running.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/admin"

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
	// One call, because an address and the credential for it are a pair. Read
	// apart they are two queries, and an edit landing between them hands the
	// new credential to the old address — during a rotation away from an
	// endpoint somebody no longer trusts, that is the replacement key
	// delivered to exactly the place it was meant to leave.
	ProvidersWithCredentials(ctx context.Context) ([]admin.ConfiguredProvider, error)
	Prices(ctx context.Context) ([]admin.ModelPrice, error)
}

// registerConfigured takes providers from the administration area, credential
// and all. This is where a key leaves the vault, and the only place it does.
func registerConfigured(ctx context.Context, registry *model.Registry, integrations *admin.Integrations) error {
	if integrations == nil {
		// No administration area at all: the environment is the whole of the
		// configuration, and it brings no rates because nothing stores any.
		registerFromEnv(registry, nil)
		return nil
	}
	failed, err := applyConfiguration(ctx, registry, integrations)
	if err != nil {
		return err
	}
	slog.Info("model providers configured", "providers", registry.Names())
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
	configured, err := from.ProvidersWithCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("read configured providers: %w", err)
	}

	failed := map[string]string{}
	ready := make([]model.Provider, 0, len(configured))
	claimed := make([]string, 0, len(configured))
	for _, p := range configured {
		// Claimed whether or not it can be used, and whether or not it is
		// switched on. Disabled means this installation said no about that
		// name; it does not mean "use whatever the environment has under it".
		claimed = append(claimed, p.Name)
		if !p.Enabled {
			continue
		}
		provider, err := providerFrom(p, priced[p.Name])
		if err != nil {
			// Reported by the caller rather than here: at boot every failure
			// is news, and on a refresh only a change is. Logging it in the
			// loop turned a process serving without a master key into two
			// lines a minute, for ever, about a state somebody had chosen.
			failed[p.Name] = err.Error()
			continue
		}
		ready = append(ready, provider)
		// Not announced here. This runs every thirty seconds in every process,
		// and a line per provider per pass is a log that says the same thing
		// four thousand times a day and buries the one line that is news. The
		// callers say it: once at boot, and afterwards only when something
		// actually moved.
		slog.Debug("provider configured", "provider", p.Name,
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
	priced, err := pricesFrom(ctx, from)
	if err != nil {
		// The layer underneath still applies, carrying whatever rates this
		// registry already holds. They are not overwritten: a refresh that
		// could not read them must not turn a priced model back into zero.
		registerFromEnv(registry, nil)
		return nil, err
	}

	// Every exit registers the environment with the rates in hand, and every
	// provider it supplies is priced as it is registered. Applied as a step
	// afterwards, a provider existed unpriced in between — and on the exit
	// below, where reading the providers failed, that step never ran at all.
	failed, err := configureFrom(ctx, registry, from, priced)
	if err != nil {
		registerFromEnv(registry, priced)
		registry.SetPrices(priced)
		return nil, err
	}

	registerFromEnv(registry, priced)
	// And the rates for what was already here: a provider the environment
	// supplied on an earlier pass is not registered again, so this is what
	// carries a rate edited since.
	registry.SetPrices(priced)
	return failed, nil
}

// providerFrom builds one provider, preset quirks and credential included.
func providerFrom(
	p admin.ConfiguredProvider, priced map[string]model.Prices,
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
		if p.Unreadable != "" {
			return model.Provider{}, fmt.Errorf("a stored credential that did not open: %s", p.Unreadable)
		}
		if p.APIKey == "" {
			// The row said a credential is stored and the read produced none,
			// with no reason attached. Registering it would build a client
			// with no key, which either borrows one from somewhere or fails at
			// the first turn.
			return model.Provider{}, errors.New("a stored credential that did not open")
		}
		provider.APIKey = p.APIKey
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
			before := registry.Revision()
			failed, err := applyConfiguration(ctx, registry, integrations)
			if err != nil && ctx.Err() == nil {
				slog.Warn("could not refresh the model configuration", "err", err)
				continue
			}
			// Only when something moved. A refresh that found the installation
			// exactly as it left it is not news, and said every thirty seconds
			// it is the thing that hides the pass that is.
			if registry.Revision() != before {
				slog.Info("model configuration refreshed", "providers", registry.Names())
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
