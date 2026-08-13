package admin

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Signing holds the key an installation signs its exports with (PRD AU-12).

One key per installation, generated the first time it is needed and kept in
the vault like every other secret. Its public half is not a secret at all —
publishing it is what makes an export checkable by somebody who does not trust
us, so it is served by an endpoint anybody may read.

Rotation is deliberately absent. A new key would invalidate every export
already in an auditor's hands, and "the signature stopped verifying because we
rotated" is indistinguishable from "the signature stopped verifying". If a key
is ever compromised the answer is a new installation identity and a statement
about which exports predate it, not a quiet swap.
*/
type Signing struct {
	pool     *pgxpool.Pool
	settings *settings.Store
}

func NewSigning(pool *pgxpool.Pool, store *settings.Store) *Signing {
	return &Signing{pool: pool, settings: store}
}

const signingName = "export_signing_key"

type storedKey struct {
	Public string `json:"public"`
}

// Key returns the installation's signing key, generating one if this is the
// first export ever asked for.
func (s *Signing) Key(ctx context.Context) (ed25519.PrivateKey, error) {
	set, err := s.settings.Reveal(ctx, settings.ScopeInstallation, domain.Scope{},
		settings.KindSigningKey, signingName)
	if err == nil && set.Secret != "" {
		seed, decodeErr := base64.StdEncoding.DecodeString(set.Secret)
		if decodeErr != nil || len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("admin: the stored signing key is unusable")
		}
		return ed25519.NewKeyFromSeed(seed), nil
	}

	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("admin: generate signing key: %w", err)
	}
	value, err := json.Marshal(storedKey{Public: base64.StdEncoding.EncodeToString(public)})
	if err != nil {
		return nil, fmt.Errorf("admin: encode signing key: %w", err)
	}

	// The seed rather than the whole private key: it is half the bytes and
	// the key is derived from it deterministically, so there is nothing extra
	// to keep secret.
	if err := writeSetting(ctx, s.pool, s.settings, "", domain.Scope{}, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      settings.KindSigningKey,
		Name:      signingName,
		Value:     value,
		Secret:    base64.StdEncoding.EncodeToString(private.Seed()),
		Enabled:   true,
	}, "export.key.created", signingName, map[string]any{
		"publicKey": base64.StdEncoding.EncodeToString(public),
	}); err != nil {
		return nil, err
	}
	return private, nil
}

// PublicKey is what an installation publishes so its exports can be checked.
func (s *Signing) PublicKey(ctx context.Context) (ed25519.PublicKey, error) {
	key, err := s.Key(ctx)
	if err != nil {
		return nil, err
	}
	return key.Public().(ed25519.PublicKey), nil
}
