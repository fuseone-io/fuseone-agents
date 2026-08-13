package auth

import (
	"context"
	"errors"
	"fmt"
	"time"
)

/*
The one-time secret that opens a fresh installation.

It is minted once, stored only as a hash, and dies the moment somebody claims
it — because the window in which an unclaimed installation will make the first
caller its owner is the most dangerous minute of this platform’s life.
*/
// storeSetupToken writes the one live setup token, replacing any earlier one.
func storeSetupToken(ctx context.Context, db execer, hash []byte, expires time.Time) error {
	if _, err := db.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, secret, enabled)
		values ('installation', '', '', 'bootstrap', 'setup_token',
		        jsonb_build_object('expires_at', $2::text), $1, true)
		on conflict (scope_kind, company_id, area_id, kind, name) do update set
			secret = excluded.secret, value = excluded.value, enabled = true`,
		hash, expires.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("auth: store setup token: %w", err)
	}
	return nil
}

// Issue mints a setup token, or reports the existing one is already out.
//
// The secret is returned once. It is printed to the process log at first
// start, which is the one channel an operator installing the platform
// definitely has — and it is why the token is single use and scoped to
// creating exactly one administrator.
func (b *Bootstrap) Issue(ctx context.Context, ttl time.Duration) (secret string, issued bool, err error) {
	pending, err := b.Pending(ctx)
	if err != nil {
		return "", false, err
	}
	if !pending {
		return "", false, ErrBootstrapClosed
	}

	var existing bool
	if err := b.pool.QueryRow(ctx, `
		select exists(select 1 from settings
		              where kind = 'bootstrap' and name = 'setup_token' and enabled)`).Scan(&existing); err != nil {
		return "", false, fmt.Errorf("auth: check setup token: %w", err)
	}
	if existing {
		// Re-printing a token every restart would leave it in every log the
		// installation ever writes. Reissuing is a deliberate command.
		return "", false, nil
	}

	token, err := NewToken()
	if err != nil {
		return "", false, err
	}
	expires := time.Now().Add(ttl)

	if err := storeSetupToken(ctx, b.pool, token.Hash, expires); err != nil {
		return "", false, err
	}

	return token.Secret, true, nil
}

// Reopen mints a setup token for an installation that already has an
// administrator, so that another one can be created.
//
// The case is not hypothetical: an installation whose only administrator loses
// their session — a person who left, an identity provider that broke, a
// browser that was cleared — cannot configure a provider, because that needs
// Curator, and cannot mint a token, because Issue refuses once an
// administrator exists. On-premise, with nobody to call, that is permanent.
//
// It requires database access, which on an installation with no working
// identity provider is the only authority there is, and it is recorded: an
// administrator appearing with no trace of how would be worse than the
// lockout it fixes. The reason is required for the same purpose — a row
// saying somebody reopened the door and not why is not an answer.
func (b *Bootstrap) Reopen(ctx context.Context, ttl time.Duration, reason string) (string, error) {
	if reason == "" {
		return "", errors.New("auth: reopening the installation requires a reason")
	}

	token, err := NewToken()
	if err != nil {
		return "", err
	}

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("auth: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := storeSetupToken(ctx, tx, token.Hash, time.Now().Add(ttl)); err != nil {
		return "", err
	}
	// In the same transaction as the token: a reopening that failed to record
	// would leave a claimable installation with nothing saying why.
	if _, err := tx.Exec(ctx, `
		insert into admin_events (principal_id, action, target, detail)
		values ('', 'bootstrap_reopened', 'installation', jsonb_build_object('reason', $1::text))`,
		reason); err != nil {
		return "", fmt.Errorf("auth: record reopening: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("auth: commit: %w", err)
	}
	return token.Secret, nil
}

// Reissue replaces the setup token. It is the escape hatch for an operator who
// lost the first one, and it deliberately requires database access — which is
// a reasonable stand-in for authority on an installation that has none yet.
func (b *Bootstrap) Reissue(ctx context.Context, ttl time.Duration) (string, error) {
	if _, err := b.pool.Exec(ctx,
		`delete from settings where kind = 'bootstrap' and name = 'setup_token'`); err != nil {
		return "", fmt.Errorf("auth: clear setup token: %w", err)
	}
	secret, _, err := b.Issue(ctx, ttl)
	return secret, err
}

// Adopt installs a caller-supplied setup token.
//
// It exists for provisioning: a chart or an installer that must know the value
// before the process starts cannot read one the process generated. The token
// is stored hashed like any other, stays single use, and the endpoint still
// closes for good the moment somebody claims it — so what this changes is who
// chose the secret, not how long it lives.
func (b *Bootstrap) Adopt(ctx context.Context, secret string, ttl time.Duration) error {
	if secret == "" {
		return errors.New("auth: a supplied setup token cannot be empty")
	}

	pending, err := b.Pending(ctx)
	if err != nil {
		return err
	}
	if !pending {
		return ErrBootstrapClosed
	}

	expires := time.Now().Add(ttl)
	if err := storeSetupToken(ctx, b.pool, HashToken(secret), expires); err != nil {
		return fmt.Errorf("auth: store supplied setup token: %w", err)
	}
	return nil
}
