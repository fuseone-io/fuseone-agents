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

/*
bootstrapGrant reaches the whole installation, deliberately.

It used to stop at the first company, on the reasoning that a company scope
reaches its areas and stops there, so it was not a superuser. That was right
until companies could be created: whoever claims an installation has to be able
to create the second company, and only the scope above them all carries that.
Anything narrower leaves a fresh installation unable to grow past the company
the bootstrap invented, with nobody who can fix it — the deadlock the setup
token exists to break, one level up.

It is what claiming means. The first person to use the setup token owns this
installation, and this is that sentence written as a grant.
*/
var bootstrapGrant = domain.Scope{Company: domain.Installation}

// Bootstrap handles the first run.
//
// A new installation is a deadlock: configuring an identity provider needs
// administrative authority, and the only ordinary way to get that authority is
// through an identity provider. The setup token breaks it exactly once.
type Bootstrap struct {
	pool *pgxpool.Pool
	dir  *Postgres
}

// execer is whatever can run a statement: the pool, or a transaction when the
// caller needs the token and its record to land together.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func NewBootstrap(pool *pgxpool.Pool, dir *Postgres) *Bootstrap {
	return &Bootstrap{pool: pool, dir: dir}
}

// Pending reports whether the installation still needs setting up.
//
// The test is whether anybody already holds installation administration. That
// is the capability the setup token exists to grant, so its presence is what
// closes the door — not a flag somebody could forget to set.
func (b *Bootstrap) Pending(ctx context.Context) (bool, error) {
	var exists bool
	err := b.pool.QueryRow(ctx,
		`select exists(select 1 from role_grants where role in ('admin', 'curator'))`).Scan(&exists)
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

// Claim exchanges the setup token for the first administrator.
//
// It creates the bootstrap scope, a local principal, and an Admin grant, then
// burns the token. Everything after this happens through the identity provider
// that administrator configures.
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
		insert into scopes (company_id, area_id, label, created_by)
		values ($1, $2, $2, 'bootstrap')
		on conflict (company_id, area_id) do nothing`,
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

	// Installation administrator, deliberately.
	//
	// The setup token exists to break the first-person deadlock: until somebody
	// can configure identity, scopes, tools and approvals, nobody else can be
	// granted the duties that separate those powers. Admin is still just a
	// scoped role; the bootstrap grant is powerful because the scope is the
	// installation.
	if _, err := tx.Exec(ctx, `
		insert into role_grants (principal_id, company_id, area_id, role, granted_by)
		values ($1, $2, $3, $4, 'bootstrap')
		on conflict do nothing`,
		principalID, string(bootstrapGrant.Company), string(bootstrapGrant.Area), string(domain.RoleAdmin)); err != nil {
		return Token{}, domain.Principal{}, fmt.Errorf("auth: grant %s: %w", domain.RoleAdmin, err)
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
			{Scope: bootstrapGrant, Role: domain.RoleAdmin},
		},
	}, nil
}
