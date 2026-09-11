package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Model providers: what an installation configured, and the credential that goes
with each.

Its own file because the questions are its own. The address and the key are a
pair and are read as one; the list of models is somebody's paste and is
bounded; and the credential leaves the vault here.
*/

// Providers lists what is configured, saying only that a credential exists.
func (i *Integrations) Providers(ctx context.Context) ([]domain.ModelProvider, error) {
	rows, err := i.settings.List(ctx, settings.KindModelProvider)
	if err != nil {
		return nil, err
	}

	out := make([]domain.ModelProvider, 0, len(rows))
	for _, row := range rows {
		var stored storedProvider
		if err := json.Unmarshal(row.Value, &stored); err != nil {
			return nil, fmt.Errorf("admin: decode provider %s: %w", row.Name, err)
		}
		out = append(out, domain.ModelProvider{
			Name: row.Name, Kind: stored.Kind, BaseURL: stored.BaseURL,
			Models:  stored.Models,
			Enabled: row.Enabled, HasKey: row.HasSecret,
			UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

/*
ConfiguredProvider is a provider and the credential that goes with it, read
together.

They travel as one value because they were read as one row. Split into two
calls, an edit between them pairs a new credential with an old address, which
is the failure a rotation exists to avoid.
*/
type ConfiguredProvider struct {
	domain.ModelProvider
	APIKey string
	// Unreadable says why a stored credential did not open here. The provider
	// is still listed: its name and its address are true whatever this process
	// can do with its key, and a caller that lost the row would let something
	// else answer under that name.
	Unreadable string
}

/*
ProvidersWithCredentials is the list a process needs to build clients.

One query, credentials opened. The ordinary listing says only that a credential
exists, which is what a screen should know; this is the wiring that has to
speak to the endpoint.
*/
func (i *Integrations) ProvidersWithCredentials(ctx context.Context) ([]ConfiguredProvider, error) {
	rows, err := i.settings.RevealAll(ctx, settings.KindModelProvider)
	if err != nil {
		return nil, err
	}
	out := make([]ConfiguredProvider, 0, len(rows))
	for _, row := range rows {
		var stored storedProvider
		if err := json.Unmarshal(row.Value, &stored); err != nil {
			return nil, fmt.Errorf("admin: decode provider %s: %w", row.Name, err)
		}
		out = append(out, ConfiguredProvider{
			ModelProvider: domain.ModelProvider{
				Name: row.Name, Kind: stored.Kind, BaseURL: stored.BaseURL,
				Models: stored.Models, Enabled: row.Enabled, HasKey: row.HasSecret,
				UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
			},
			APIKey:     row.Secret,
			Unreadable: row.SecretUnreadable,
		})
	}
	return out, nil
}

// PutProvider records a provider. An empty key keeps the stored one, so
// changing a base URL does not require re-entering a credential — which is how
// operators end up pasting keys into chat to look them up.
func (i *Integrations) PutProvider(ctx context.Context, by domain.UserID, scope domain.Scope, provider domain.ModelProvider, apiKey string) error {
	switch {
	case strings.TrimSpace(provider.Name) == "":
		return ErrNoName
	// Required only where the client cannot already know it. Anthropic's does;
	// an OpenAI-compatible endpoint, including a self-hosted one, is known
	// only to the installation. Demanding it from everybody asked for a value
	// nobody has and made the reference provider impossible to configure.
	case provider.Kind != "anthropic" && strings.TrimSpace(provider.BaseURL) == "":
		return ErrNoBaseURL
	}

	models, err := cleanModels(provider.Models)
	if err != nil {
		return err
	}
	value, err := json.Marshal(storedProvider{
		Kind: provider.Kind, BaseURL: provider.BaseURL, Models: models,
	})
	if err != nil {
		return fmt.Errorf("admin: encode provider: %w", err)
	}

	return writeSetting(ctx, i.pool, i.settings, by, scope, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      settings.KindModelProvider,
		Name:      provider.Name,
		Value:     value,
		Secret:    apiKey,
		Enabled:   provider.Enabled,
		UpdatedBy: string(by),
	}, "provider.configured", provider.Name, map[string]any{
		// The credential is never in the trail, only the fact that one arrived.
		"kind": provider.Kind, "baseURL": provider.BaseURL,
		"enabled": provider.Enabled, "keyChanged": apiKey != "",
	})
}

// MaxProviderModels and MaxModelName bound a list somebody pastes.
//
// Not a judgement about how many models an endpoint may serve — it is that this
// list is decoded by every process every thirty seconds, and a paste nobody
// meant to make would be paid for on every refresh, for ever, by an
// installation that cannot see why.
const (
	MaxProviderModels = 200
	MaxModelName      = 200
)

// ErrTooManyModels, ErrModelNameTooLong and ErrModelNameNotOneLine refuse a
// list that is not a list.
var (
	ErrTooManyModels       = errors.New("admin: more models than a provider list may hold")
	ErrModelNameTooLong    = errors.New("admin: a model name longer than any vendor uses")
	ErrModelNameNotOneLine = errors.New("admin: a model name containing a line break")
)

/*
cleanModels drops blanks and repeats from a list somebody typed.

Trimmed rather than refused: a trailing line or a stray space is not a mistake
worth stopping a save for, and an empty entry stored would show as a nameless
suggestion in the one control that exists to stop people guessing names.

A list past the bounds is refused rather than truncated. Silently keeping the
first two hundred of somebody's paste is the platform deciding which of their
models exist.
*/
func cleanModels(in []string) ([]string, error) {
	if len(in) > MaxProviderModels {
		return nil, fmt.Errorf("%w: %d, and the limit is %d",
			ErrTooManyModels, len(in), MaxProviderModels)
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, one := range in {
		one = strings.TrimSpace(one)
		if one == "" || seen[one] {
			continue
		}
		// Counted in characters, because the contract says characters. Measured
		// in bytes, a name of two hundred perfectly ordinary Unicode characters
		// was refused and told it was three hundred.
		if utf8.RuneCountInString(one) > MaxModelName {
			// The name is not in the error. It came from a paste that may be
			// anything, and a refusal that quotes its input is a refusal that
			// can be made to say whatever the sender chose.
			return nil, fmt.Errorf("%w: %d characters, and the limit is %d",
				ErrModelNameTooLong, utf8.RuneCountInString(one), MaxModelName)
		}
		// A name is one line. The console edits this list as lines, so a name
		// carrying a break comes back as two names the next time anybody saves
		// anything on that screen — a round trip that quietly rewrites a
		// configuration nobody touched.
		if strings.ContainsAny(one, "\r\n") {
			return nil, ErrModelNameNotOneLine
		}
		seen[one] = true
		out = append(out, one)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (i *Integrations) DeleteProvider(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error {
	return removeSetting(ctx, i.pool, i.settings, by, scope, settings.KindModelProvider, name, "provider.removed")
}

// Credential opens a provider's key. Separate and explicit: reading
// configuration is routine, reading a credential is not.
func (i *Integrations) Credential(ctx context.Context, name string) (string, error) {
	set, err := i.settings.Reveal(ctx, settings.ScopeInstallation, domain.Scope{}, settings.KindModelProvider, name)
	if err != nil {
		return "", err
	}
	return set.Secret, nil
}
