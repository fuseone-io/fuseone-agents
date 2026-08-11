package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// execer is whatever can run a statement: the pool, or a transaction when the
// caller needs the token and its record to land together.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

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

// Open reports whether somebody holding a setup token can still claim.
//
// This is the question the login screen asks, and it is not the same as
// Pending: a reopened installation already has an administrator and still has
// to show the setup form, or the operator holding a fresh token has nowhere to
// type it.
func (b *Bootstrap) Open(ctx context.Context) (bool, error) {
	var open bool
	err := b.pool.QueryRow(ctx, `
		select exists(select 1 from settings
		              where kind = 'bootstrap' and name = 'setup_token' and enabled
		                and (value->>'expires_at')::timestamptz > now())`).Scan(&open)
	if err != nil {
		return false, fmt.Errorf("auth: check setup token: %w", err)
	}
	return open, nil
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

// Claim exchanges the setup token for the first administrator.
//
// It creates the bootstrap scope, a local principal, and a Curator grant, then
// burns the token. Everything after this happens through the identity
// provider that administrator configures.
func (b *Bootstrap) Claim(ctx context.Context, secret, display, userAgent, ip string) (Token, domain.Principal, error) {
	// The token is the credential, and its existence is the gate.
	//
	// This used to also require that no administrator existed, which read as
	// a second lock but was really a trapdoor: claiming burns the token, so a
	// claimed installation has none and the door is shut anyway. All the
	// extra check did was make Reopen impossible, and with it any recovery
	// from an installation whose only administrator can no longer get in.
	var (
		stored  []byte
		valueJS map[string]string
	)
	err := b.pool.QueryRow(ctx, `
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

	// Every role, deliberately.
	//
	// The four roles exist to separate duties — the Curator writes the rules
	// and the Approver decides on exceptions to them — and that separation is
	// only meaningful once there is more than one person. Until then it is a
	// deadlock: an installation whose sole administrator cannot approve
	// anything has an approval queue nobody can empty, which is the same shape
	// as the deadlock the setup token exists to break.
	//
	// Narrowing this is the first thing an operator should do after
	// configuring an identity provider, and the trail records that it started
	// here.
	for _, role := range []domain.Role{
		domain.RoleCurator, domain.RoleApprover, domain.RoleAuthor, domain.RoleAuditor,
	} {
		if _, err := tx.Exec(ctx, `
			insert into role_grants (principal_id, company_id, area_id, role, granted_by)
			values ($1, $2, $3, $4, 'bootstrap')
			on conflict do nothing`,
			principalID, string(bootstrapGrant.Company), string(bootstrapGrant.Area), string(role)); err != nil {
			return Token{}, domain.Principal{}, fmt.Errorf("auth: grant %s: %w", role, err)
		}
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
		Grants: []domain.Grant{
			{Scope: bootstrapGrant, Role: domain.RoleCurator},
			{Scope: bootstrapGrant, Role: domain.RoleApprover},
			{Scope: bootstrapGrant, Role: domain.RoleAuthor},
			{Scope: bootstrapGrant, Role: domain.RoleAuditor},
		},
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
	if err := storeSetupToken(ctx, b.pool, HashToken(secret), expires); err != nil {
		return fmt.Errorf("auth: store supplied setup token: %w", err)
	}
	return nil
}
