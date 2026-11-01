package trigger

import (
	"fmt"

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
