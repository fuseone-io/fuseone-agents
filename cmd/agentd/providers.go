// Command agentd is the FuseOne Agents server.
//
// One binary, one Postgres, nothing else required (PRD DE-01). Subcommands
// select the role a process plays inside the installation.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/fuseone/agents/internal/vault"

	"github.com/fuseone/agents/internal/admin"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/policy"
)

// Model providers and the secrets they need.

// policySource is where the set comes from, or a source of nothing when this
// worker has no database. An installation running on the in-memory ledger
// decides under the built-in ladder, which is the safe default rather than an
// absence of rules.
func policySource(pool *pgxpool.Pool) policy.Source {
	if pool == nil {
		return emptyPolicies{}
	}
	return policy.NewStore(pool)
}

type emptyPolicies struct{}

func (emptyPolicies) Active(context.Context) (policy.Set, error) {
	return policy.Set{Hash: "builtin", Policies: nil}, nil
}

// registerConfigured takes providers from the administration area, credential
// and all. This is where a key leaves the vault, and the only place it does.
func registerConfigured(ctx context.Context, registry *model.Registry, integrations *admin.Integrations) error {
	if integrations == nil {
		return nil
	}

	configured, err := integrations.Providers(ctx)
	if err != nil {
		return fmt.Errorf("read configured providers: %w", err)
	}

	// The installation's own rates, attached to the provider they belong to.
	// Read once here rather than at each call: a run and an authoring call
	// build their clients through different paths, and a price fetched per
	// call is a price one of those paths forgets.
	rates, err := integrations.Prices(ctx)
	if err != nil {
		return fmt.Errorf("read configured prices: %w", err)
	}
	priced := map[string]map[string]model.Prices{}
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

	for _, p := range configured {
		if !p.Enabled {
			continue
		}
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
			key, err := integrations.Credential(ctx, p.Name)
			if err != nil {
				return fmt.Errorf("open credential for %s: %w", p.Name, err)
			}
			provider.APIKey = key
		}
		provider.Prices = priced[p.Name]
		if err := registry.Register(provider); err != nil {
			return err
		}
		slog.Info("provider configured", "provider", p.Name,
			"source", "administration", "priced_models", len(provider.Prices))
	}
	return nil
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

// keygen prints a master key.
//
// It goes to stdout and nowhere else: the key is never stored by the platform,
// because a platform that can read its own credentials at rest offers an
// attacker with database access nothing to break.
func keygen() error {
	key, err := vault.GenerateKey()
	if err != nil {
		return err
	}
	fmt.Println(key)
	fmt.Fprintf(os.Stderr,
		"\nSet this as %s wherever agentd runs. Losing it means every stored\n"+
			"credential has to be entered again; leaking it means they are all readable.\n",
		vault.KeyEnv)
	return nil
}

// openVault reads the master key. Configuration with a credential in it is
// unreadable without one, so a worker that needs providers needs this.
func openVault() (*vault.Vault, error) {
	// The key id travels with the ciphertext so a future rotation can tell
	// which key sealed a given row.
	v, err := vault.FromEnv("primary")
	if errors.Is(err, vault.ErrNoKey) {
		// Wrapped, so the sentinel survives. The console serves without a key
		// and the worker refuses to start without one, and both decide by
		// asking errors.Is — a message that only reads like the sentinel
		// makes "no key" indistinguishable from "a key that is wrong", which
		// is how a first install stops booting at all.
		return nil, fmt.Errorf(
			"%w: the administration area seals credentials; set %s (agentd version prints how)",
			vault.ErrNoKey, vault.KeyEnv)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", vault.KeyEnv, err)
	}
	return v, nil
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
