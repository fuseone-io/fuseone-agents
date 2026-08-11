package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

var (
	// ErrBootstrapClosed means the installation already has an administrator.
	// Once that is true the setup path is gone for good — an open bootstrap
	// endpoint on a running installation is a way in.
	ErrBootstrapClosed = errors.New("auth: this installation is already set up")
	ErrNoBootstrap     = errors.New("auth: no setup token has been issued")
)

// BootstrapScope is where the first administrator is granted.
//
// A fresh installation has no companies or areas yet, and somebody has to be
// able to create them. This scope is created alongside the first grant and is
// an ordinary scope afterwards — nothing about it stays privileged.
var BootstrapScope = domain.Scope{Company: "default", Area: "platform"}

// bootstrapGrant is company-wide, deliberately.
//
// Granting the first administrator only the platform area would leave them
// unable to see anything in the areas they are about to create — the grant
// would be made before its subject existed. A company scope reaches its areas
// (PRD §3.1) and stops at the company, so this is not a superuser: a second
// company in phase 2 is invisible to it.
var bootstrapGrant = domain.Scope{Company: BootstrapScope.Company}

// Bootstrap handles the first run.
//
// A new installation is a deadlock: configuring an identity provider needs the
// Curator permission, and the only way to get a Curator is through an identity
// provider. The setup token breaks it exactly once.
type Bootstrap struct {
	pool *pgxpool.Pool
	dir  *Postgres
}

func NewBootstrap(pool *pgxpool.Pool, dir *Postgres) *Bootstrap {
	return &Bootstrap{pool: pool, dir: dir}
}

// Pending reports whether the installation still needs setting up.
//
// The test is whether anybody holds Curator anywhere. That is the capability
// the setup token exists to grant, so its presence is what closes the door —
// not a flag somebody could forget to set.
func (b *Bootstrap) Pending(ctx context.Context) (bool, error) {
	var exists bool
	err := b.pool.QueryRow(ctx,
		`select exists(select 1 from role_grants where role = 'curator')`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("auth: check bootstrap: %w", err)
	}
	return !exists, nil
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

	if _, err := b.pool.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, secret, enabled)
		values ('installation', '', '', 'bootstrap', 'setup_token',
		        jsonb_build_object('expires_at', $2::text), $1, true)
		on conflict (scope_kind, company_id, area_id, kind, name) do update set
			secret = excluded.secret, value = excluded.value, enabled = true`,
		token.Hash, expires.Format(time.RFC3339)); err != nil {
		return "", false, fmt.Errorf("auth: store setup token: %w", err)
	}

	return token.Secret, true, nil
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

// Claim exchanges the setup token for the first administrator.
//
// It creates the bootstrap scope, a local principal, and a Curator grant, then
// burns the token. Everything after this happens through the identity
// provider that administrator configures.
func (b *Bootstrap) Claim(ctx context.Context, secret, display, userAgent, ip string) (Token, domain.Principal, error) {
	pending, err := b.Pending(ctx)
	if err != nil {
		return Token{}, domain.Principal{}, err
	}
	if !pending {
		return Token{}, domain.Principal{}, ErrBootstrapClosed
	}

	var (
		stored  []byte
		valueJS map[string]string
	)
	err = b.pool.QueryRow(ctx, `
		select secret, jsonb_object_agg(k, v)
		from settings, lateral jsonb_each_text(value) as e(k, v)
		where kind = 'bootstrap' and name = 'setup_token' and enabled
		group by secret`).Scan(&stored, &valueJS)

	if errors.Is(err, pgx.ErrNoRows) {
		return Token{}, domain.Principal{}, ErrNoBootstrap
	}
	if err != nil {
		return Token{}, domain.Principal{}, fmt.Errorf("auth: read setup token: %w", err)
	}

	if !EqualHash(stored, HashToken(secret)) {
		return Token{}, domain.Principal{}, ErrBadCredential
	}
	if raw, ok := valueJS["expires_at"]; ok {
		if expires, perr := time.Parse(time.RFC3339, raw); perr == nil && time.Now().After(expires) {
			return Token{}, domain.Principal{}, ErrExpired
		}
	}

	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return Token{}, domain.Principal{}, fmt.Errorf("auth: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The scope has to exist before anything can be granted in it.
	if _, err := tx.Exec(ctx, `
		insert into companies (company_id, name) values ($1, $1)
		on conflict (company_id) do nothing`, string(BootstrapScope.Company)); err != nil {
		return Token{}, domain.Principal{}, fmt.Errorf("auth: create company: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		insert into areas (company_id, area_id, name) values ($1, $2, $2)
		on conflict do nothing`,
		string(BootstrapScope.Company), string(BootstrapScope.Area)); err != nil {
		return Token{}, domain.Principal{}, fmt.Errorf("auth: create area: %w", err)
	}

	if display == "" {
		display = "Primeiro administrador"
	}
	principalID := "usr_" + shortHash("bootstrap|"+display)

	if _, err := tx.Exec(ctx, `
		insert into principals (principal_id, kind, provider, subject, display, last_seen_at)
		values ($1, 'user', 'bootstrap', $1, $2, now())
		on conflict (principal_id) do update set last_seen_at = now()`,
		principalID, display); err != nil {
		return Token{}, domain.Principal{}, fmt.Errorf("auth: create principal: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		insert into role_grants (principal_id, company_id, area_id, role, granted_by)
		values ($1, $2, $3, 'curator', 'bootstrap')
		on conflict do nothing`,
		principalID, string(bootstrapGrant.Company), string(bootstrapGrant.Area)); err != nil {
		return Token{}, domain.Principal{}, fmt.Errorf("auth: grant curator: %w", err)
	}

	// Burn the token in the same transaction that grants the role: a partial
	// bootstrap that leaves a live token is worse than one that fails outright.
	if _, err := tx.Exec(ctx,
		`delete from settings where kind = 'bootstrap' and name = 'setup_token'`); err != nil {
		return Token{}, domain.Principal{}, fmt.Errorf("auth: burn setup token: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		insert into admin_events (principal_id, company_id, area_id, action, target)
		values ($1, $2, $3, 'bootstrap.claim', $1)`,
		principalID, string(BootstrapScope.Company), string(BootstrapScope.Area)); err != nil {
		return Token{}, domain.Principal{}, fmt.Errorf("auth: record bootstrap: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Token{}, domain.Principal{}, fmt.Errorf("auth: commit bootstrap: %w", err)
	}

	session, _, err := b.dir.CreateSession(ctx, principalID, userAgent, ip, time.Now())
	if err != nil {
		return Token{}, domain.Principal{}, err
	}

	return session, domain.Principal{
		ID:      domain.UserID(principalID),
		Subject: principalID,
		Display: display,
		Kind:    domain.PrincipalUser,
		Grants:  []domain.Grant{{Scope: bootstrapGrant, Role: domain.RoleCurator}},
	}, nil
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
	if _, err := b.pool.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, secret, enabled)
		values ('installation', '', '', 'bootstrap', 'setup_token',
		        jsonb_build_object('expires_at', $2::text), $1, true)
		on conflict (scope_kind, company_id, area_id, kind, name) do update set
			secret = excluded.secret, value = excluded.value, enabled = true`,
		HashToken(secret), expires.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("auth: store supplied setup token: %w", err)
	}
	return nil
}
