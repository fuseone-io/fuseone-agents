package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// ErrNoIssuer means a provider was configured with nothing to discover.
var ErrNoIssuer = errors.New("admin: an identity provider needs an issuer")

/*
Identity is who may sign in, and what signing in grants.

The two are deliberately separate. A provider proves who somebody is; the
mappings decide what that makes them, and a provider with no mapping grants
nothing however successfully somebody authenticates. An installation that
wired the two together would hand every person its identity provider knows
about whatever the first mapping happened to say.
*/
type Identity struct {
	pool     *pgxpool.Pool
	settings *settings.Store
}

func NewIdentity(pool *pgxpool.Pool, store *settings.Store) *Identity {
	return &Identity{pool: pool, settings: store}
}

// storedProviderConfig is the shape kept in the setting's value. The client
// secret is not in it: that goes to the vault, under the same seal as every
// other credential.
type storedProviderConfig struct {
	Display     string                `json:"display"`
	Issuer      string                `json:"issuer"`
	ClientID    string                `json:"clientId"`
	GroupsClaim string                `json:"groupsClaim,omitempty"`
	Mappings    []domain.GroupMapping `json:"mappings,omitempty"`
}

// IdentityProviders lists what is configured, without any credential.
func (i *Identity) IdentityProviders(ctx context.Context) ([]domain.IdentityProvider, error) {
	found, err := i.settings.List(ctx, settings.KindIdentityProvider)
	if err != nil {
		return nil, err
	}

	out := make([]domain.IdentityProvider, 0, len(found))
	for _, set := range found {
		var cfg storedProviderConfig
		if err := json.Unmarshal(set.Value, &cfg); err != nil {
			return nil, fmt.Errorf("admin: decode identity provider %s: %w", set.Name, err)
		}
		out = append(out, domain.IdentityProvider{
			ID: set.Name, Display: cfg.Display, Issuer: cfg.Issuer,
			ClientID: cfg.ClientID, GroupsClaim: cfg.GroupsClaim, Mappings: cfg.Mappings,
			HasSecret: set.HasSecret, Enabled: set.Enabled,
			UpdatedBy: set.UpdatedBy, UpdatedAt: set.UpdatedAt,
		})
	}
	return out, nil
}

// PutIdentityProvider stores one, creating or replacing.
//
// An empty secret keeps whichever one is stored. Changing a group mapping must
// not demand re-entering a credential nobody has to hand — the alternative is
// an operator leaving the mapping wrong because fixing it costs more than they
// can pay right then.
func (i *Identity) PutIdentityProvider(
	ctx context.Context, by domain.UserID, scope domain.Scope,
	provider domain.IdentityProvider, clientSecret string,
) error {
	switch {
	case strings.TrimSpace(provider.ID) == "":
		return ErrNoName
	case strings.TrimSpace(provider.Issuer) == "":
		// Discovery is what makes an assertion verifiable, and it is the only
		// thing that can fail where somebody is looking. Without an issuer the
		// failure moves to sign-in, where they are not.
		return ErrNoIssuer
	}

	value, err := json.Marshal(storedProviderConfig{
		Display: provider.Display, Issuer: provider.Issuer, ClientID: provider.ClientID,
		GroupsClaim: provider.GroupsClaim, Mappings: provider.Mappings,
	})
	if err != nil {
		return fmt.Errorf("admin: encode identity provider: %w", err)
	}

	return writeSetting(ctx, i.pool, i.settings, by, scope, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      settings.KindIdentityProvider,
		Name:      provider.ID,
		Value:     value,
		Secret:    clientSecret,
		Enabled:   provider.Enabled,
		UpdatedBy: string(by),
	}, "identity.provider.configured", provider.ID, map[string]any{
		// Never the secret, only that one arrived. Who may sign in and what it
		// grants them is the most consequential thing on this screen, so the
		// mappings themselves are in the trail.
		"issuer": provider.Issuer, "clientId": provider.ClientID,
		"enabled": provider.Enabled, "secretChanged": clientSecret != "",
		"mappings": provider.Mappings,
	})
}

func (i *Identity) DeleteIdentityProvider(
	ctx context.Context, by domain.UserID, scope domain.Scope, id string,
) error {
	return removeSetting(ctx, i.pool, i.settings, by, scope, settings.KindIdentityProvider, id, "identity.provider.removed")
}

// IdentitySecret opens a provider's client secret. Separate and explicit:
// reading configuration is routine, reading a credential is not.
func (i *Identity) IdentitySecret(ctx context.Context, id string) (string, error) {
	set, err := i.settings.Reveal(ctx, settings.ScopeInstallation, domain.Scope{}, settings.KindIdentityProvider, id)
	if err != nil {
		return "", err
	}
	return set.Secret, nil
}
