package trigger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// Offer is what a delivery brings to prove itself: the secret an ordinary
// hook's caller sends, or the signature GitHub computes over the body.
type Offer struct {
	Secret    string
	Signature string
	Body      []byte
}

// ErrRejected means the hook exists and is armed, and the offer does not
// prove it. The door answers it exactly as it answers ErrNoHook.
var ErrRejected = errors.New("trigger: the delivery does not prove the hook")

/*
Authenticate returns the hook at path if the offer proves it, from one read.

The row whose key is checked is the row returned: a check on one read and a
lookup on another let a delivery authenticated against one agent's row open a
run for whichever agent held the path a moment later.

The row's own source decides what proves it. An ordinary hook is proved by
the sent secret, compared in constant time with the hash; a GitHub hook only
by the signature over the body, its sealed secret opened for this one check.
A GitHub hook's secret sent as if it were an ordinary one proves nothing.
*/
func (p *PostgresWebhooks) Authenticate(ctx context.Context, path string, offer Offer) (Hook, error) {
	var nonce []byte
	row := p.pool.QueryRow(ctx,
		`select `+hookColumns+`, secret_hash, secret_nonce from webhook_triggers where path = $1`, path)
	hook, cred, err := scanCredential(row, &nonce)
	if errors.Is(err, pgx.ErrNoRows) {
		return Hook{}, ErrNoHook
	}
	if err != nil {
		return Hook{}, fmt.Errorf("trigger: authenticate %q: %w", path, err)
	}
	if !hook.Armed {
		return Hook{}, ErrNotArmed
	}
	if hook.From != domain.WebhookFromGitHub {
		if !MatchesSecret(cred.hash, offer.Secret) {
			return Hook{}, ErrRejected
		}
		return hook, nil
	}
	if p.vault == nil {
		return Hook{}, ErrNoVault
	}
	secret, err := p.vault.Open(cred.sealed, nonce, sealContext(path))
	if err != nil {
		return Hook{}, fmt.Errorf("trigger: open the signing secret of %q: %w", path, err)
	}
	if !VerifyGitHubSignature(secret, offer.Body, offer.Signature) {
		return Hook{}, ErrRejected
	}
	return hook, nil
}

// credential is the half of a row that proves a delivery.
type credential struct{ hash, sealed []byte }

// scanCredential reads hookColumns followed by secret_hash and secret_nonce.
func scanCredential(row pgx.Row, nonce *[]byte) (Hook, credential, error) {
	var cred credential
	hook, err := scanHookWith(row, &cred.sealed, &cred.hash, nonce)
	return hook, cred, err
}
