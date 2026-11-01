package trigger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// Sealer is the vault, as a GitHub hook needs it: its secret is read back to
// check signatures, so it is kept encrypted rather than hashed.
type Sealer interface {
	Seal(secret []byte, context string) (ciphertext, nonce []byte, err error)
	Open(ciphertext, nonce []byte, context string) ([]byte, error)
}

// WithVault lets GitHub hooks be armed. Without it, rotating one is refused
// with ErrNoVault and it stays closed; ordinary hooks never need it.
func (p *PostgresWebhooks) WithVault(v Sealer) *PostgresWebhooks {
	p.vault = v
	return p
}

// sealContext binds a sealed secret to its path, so a row copied onto another
// path does not open there.
func sealContext(path string) string { return "webhook_triggers|" + path }

// sealFor seals the secret of a GitHub hook, and nothing for an ordinary one.
func (p *PostgresWebhooks) sealFor(source, path, secret string) (sealed, nonce []byte, err error) {
	if source != domain.WebhookFromGitHub {
		return nil, nil, nil
	}
	if p.vault == nil {
		return nil, nil, ErrNoVault
	}
	sealed, nonce, err = p.vault.Seal([]byte(secret), sealContext(path))
	if err != nil {
		return nil, nil, fmt.Errorf("trigger: seal the secret of %q: %w", path, err)
	}
	return sealed, nonce, nil
}

// SigningSecret opens a GitHub hook's secret.
//
// The error never carries the secret, the ciphertext or the vault's reason: a
// failure to open is reported as a failure, not as an oracle.
func (p *PostgresWebhooks) SigningSecret(ctx context.Context, path string) ([]byte, error) {
	var (
		source        string
		hash          []byte
		sealed, nonce []byte
	)
	err := p.pool.QueryRow(ctx, `
		select source, secret_hash, secret_sealed, secret_nonce
		from webhook_triggers where path = $1`, path).Scan(&source, &hash, &sealed, &nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoHook
	}
	if err != nil {
		return nil, fmt.Errorf("trigger: read the signing secret of %q: %w", path, err)
	}
	switch {
	case source != domain.WebhookFromGitHub:
		return nil, ErrNotGitHub
	case len(hash) == 0 || len(sealed) == 0:
		return nil, ErrNotArmed
	case p.vault == nil:
		return nil, ErrNoVault
	}
	secret, err := p.vault.Open(sealed, nonce, sealContext(path))
	if err != nil {
		return nil, fmt.Errorf("trigger: open the signing secret of %q: %w", path, err)
	}
	return secret, nil
}
