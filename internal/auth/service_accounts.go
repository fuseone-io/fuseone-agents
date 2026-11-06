package auth

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

var (
	ErrServiceAccountName     = errors.New("auth: a service account name is lowercase letters, digits and hyphens, starting with a letter")
	ErrServiceAccountTaken    = errors.New("auth: that service account name is taken")
	ErrNoServiceAccount       = errors.New("auth: no such service account")
	ErrNoPrincipal            = errors.New("auth: no such principal")
	ErrServiceAccountDisabled = errors.New("auth: this service account is disabled")
	ErrNoServiceToken         = errors.New("auth: no such token")
	ErrTokenExpiry            = errors.New("auth: a token needs an expiry in the future")
)

var serviceAccountName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// IssuedToken is the one moment a secret exists outside the caller's hands.
type IssuedToken struct {
	domain.ServiceToken
	Secret string
}

// CreateServiceAccount records a machine that holds nothing yet. Granting is the next
// act and a separate line on the trail, as it is for a person.
func (p *Postgres) CreateServiceAccount(
	ctx context.Context, name, description, by string,
) (domain.ServiceAccount, error) {
	if !serviceAccountName.MatchString(name) {
		return domain.ServiceAccount{}, ErrServiceAccountName
	}
	account := domain.ServiceAccount{ID: "svc_" + name, Name: name, Description: description}
	err := p.onTrail(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			insert into principals (principal_id, kind, provider, subject, display, description)
			values ($1, 'service', 'service', $2, $2, $3)
			returning created_at`, account.ID, name, description).Scan(&account.CreatedAt)
		if isUniqueViolation(err) {
			return ErrServiceAccountTaken
		}
		if err != nil {
			return fmt.Errorf("auth: create service account: %w", err)
		}
		return recordIdentity(ctx, tx, by, "service_account.created", account.ID, "")
	})
	if err != nil {
		return domain.ServiceAccount{}, err
	}
	return account, nil
}

// SetServiceAccountDisabled turns every token of an account off, or back on. A read of
// PrincipalByToken checks it, so it takes effect on the next request.
func (p *Postgres) SetServiceAccountDisabled(
	ctx context.Context, id string, disabled bool, by string, now time.Time,
) error {
	action, at := "service_account.enabled", (*time.Time)(nil)
	if disabled {
		action, at = "service_account.disabled", &now
	}
	return p.onTrail(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			update principals set disabled_at = $2
			where principal_id = $1 and kind = 'service'`, id, at)
		if err != nil {
			return fmt.Errorf("auth: set service account state: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNoServiceAccount
		}
		return recordIdentity(ctx, tx, by, action, id, "")
	})
}

// ServiceAccounts lists every machine, with what it holds.
func (p *Postgres) ServiceAccounts(ctx context.Context) ([]domain.ServiceAccount, error) {
	rows, err := p.pool.Query(ctx, `
		select p.principal_id, p.subject, p.description, p.disabled_at is not null, p.created_at,
		       coalesce(g.company_id, ''), coalesce(g.area_id, ''), coalesce(g.role, '')
		from principals p
		left join role_grants g on g.principal_id = p.principal_id
		where p.kind = 'service'
		order by p.subject, g.company_id, g.area_id, g.role`)
	if err != nil {
		return nil, fmt.Errorf("auth: list service accounts: %w", err)
	}
	defer rows.Close()

	var out []domain.ServiceAccount
	for rows.Next() {
		var account domain.ServiceAccount
		var company, area, role string
		if err := rows.Scan(&account.ID, &account.Name, &account.Description, &account.Disabled,
			&account.CreatedAt, &company, &area, &role); err != nil {
			return nil, fmt.Errorf("auth: scan service account: %w", err)
		}
		if len(out) == 0 || out[len(out)-1].ID != account.ID {
			out = append(out, account)
		}
		if role != "" {
			last := &out[len(out)-1]
			last.Grants = append(last.Grants, domain.Grant{
				Scope: domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)},
				Role:  domain.Role(role),
			})
		}
	}
	return out, rows.Err()
}

/*
IssueServiceToken mints a credential for a service account and returns its secret, the
one time it is ever available.

Only for a service account. A token on a person would be a way to act as them with no
sign-in, so this refuses, in the same statement that reads the account. An expiry is
required: a credential nobody chose a lifetime for is one that outlives whoever made it.
*/
func (p *Postgres) IssueServiceToken(
	ctx context.Context, account, name, by string, expires, now time.Time,
) (IssuedToken, error) {
	if expires.IsZero() || !now.Before(expires) {
		return IssuedToken{}, ErrTokenExpiry
	}
	secret, err := NewToken()
	if err != nil {
		return IssuedToken{}, err
	}
	issued := IssuedToken{Secret: secret.Secret, ServiceToken: domain.ServiceToken{
		// From the secret's hash, so two with one name are two tokens: rotating is
		// issuing beside the old one, and a person will reuse the name.
		ID:   "tok_" + hex.EncodeToString(secret.Hash[:8]),
		Name: name, CreatedBy: by, ExpiresAt: expires,
	}}
	err = p.onTrail(ctx, func(tx pgx.Tx) error {
		if err := requireEnabledServiceAccount(ctx, tx, account); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			insert into api_tokens (token_id, principal_id, name, token_hash, created_by, created_at, expires_at)
			values ($1, $2, $3, $4, $5, $6, $7)
			returning created_at`, issued.ID, account, name, secret.Hash, by, now, expires,
		).Scan(&issued.CreatedAt); err != nil {
			return fmt.Errorf("auth: issue token: %w", err)
		}
		return recordIdentity(ctx, tx, by, "service_account.token_issued", account, issued.ID)
	})
	if err != nil {
		return IssuedToken{}, err
	}
	return issued, nil
}

func requireEnabledServiceAccount(ctx context.Context, tx pgx.Tx, account string) error {
	var disabled bool
	err := tx.QueryRow(ctx, `
		select disabled_at is not null from principals
		where principal_id = $1 and kind = 'service' for update`, account).Scan(&disabled)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNoServiceAccount
	case err != nil:
		return fmt.Errorf("auth: read service account: %w", err)
	case disabled:
		return ErrServiceAccountDisabled
	}
	return nil
}

// ServiceTokens lists an account's tokens, never their secrets.
func (p *Postgres) ServiceTokens(ctx context.Context, account string) ([]domain.ServiceToken, error) {
	rows, err := p.pool.Query(ctx, `
		select t.token_id, t.name, t.created_by, t.created_at, t.expires_at, t.last_used_at, t.revoked_at
		from api_tokens t
		join principals p on p.principal_id = t.principal_id and p.kind = 'service'
		where t.principal_id = $1
		order by t.created_at desc, t.token_id`, account)
	if err != nil {
		return nil, fmt.Errorf("auth: list tokens: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.ServiceToken, error) {
		var token domain.ServiceToken
		err := row.Scan(&token.ID, &token.Name, &token.CreatedBy, &token.CreatedAt,
			&token.ExpiresAt, &token.LastUsedAt, &token.RevokedAt)
		return token, err
	})
}

// RevokeServiceToken takes effect on the next request, because PrincipalByToken reads
// it every time. Revoking a revoked token is asking for a state and is not an error,
// and the first revocation is the one that keeps its date.
func (p *Postgres) RevokeServiceToken(
	ctx context.Context, account, token, by string, now time.Time,
) error {
	return p.onTrail(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			update api_tokens t set revoked_at = coalesce(t.revoked_at, $3)
			from principals p
			where t.principal_id = p.principal_id and p.kind = 'service'
			  and t.principal_id = $1 and t.token_id = $2`, account, token, now)
		if err != nil {
			return fmt.Errorf("auth: revoke token: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNoServiceToken
		}
		return recordIdentity(ctx, tx, by, "service_account.token_revoked", account, token)
	})
}

// onTrail runs an identity change and its record as one act: a change with no record
// is the thing an auditor comes looking for.
func (p *Postgres) onTrail(ctx context.Context, change func(pgx.Tx) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := change(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("auth: commit: %w", err)
	}
	return nil
}

func recordIdentity(ctx context.Context, tx pgx.Tx, by, action, target, token string) error {
	if _, err := tx.Exec(ctx, `
		insert into admin_events (principal_id, action, target, detail)
		values ($1, $2, $3, jsonb_strip_nulls(jsonb_build_object('token', nullif($4::text, ''))))`,
		by, action, target, token); err != nil {
		return fmt.Errorf("auth: record %s: %w", action, err)
	}
	return nil
}
