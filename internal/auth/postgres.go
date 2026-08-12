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

// SessionTTL is how long a browser session lives before the operator signs in
// again. Long enough not to interrupt a working day, short enough that a
// forgotten laptop stops being useful overnight.
const SessionTTL = 12 * time.Hour

// Postgres stores identities, grants, sessions and API tokens.
type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

var _ Directory = (*Postgres)(nil)

// PrincipalBySession resolves a live session and records that it was used.
//
// An expired session reports ErrExpired rather than ErrBadCredential: the
// console sends someone to sign in again for one and shows a failure for the
// other, and collapsing them makes a normal lapse look like a bug.
func (p *Postgres) PrincipalBySession(ctx context.Context, tokenHash []byte, now time.Time) (domain.Principal, Session, error) {
	var (
		s          Session
		revoked    *time.Time
		disabled   *time.Time
		kind, disp string
		subject    string
	)

	err := p.pool.QueryRow(ctx, `
		select s.session_id, s.principal_id, s.created_at, s.expires_at, s.revoked_at,
		       pr.kind, pr.display, pr.subject, pr.disabled_at
		from sessions s
		join principals pr on pr.principal_id = s.principal_id
		where s.token_hash = $1`, tokenHash,
	).Scan(&s.ID, &s.PrincipalID, &s.CreatedAt, &s.ExpiresAt, &revoked,
		&kind, &disp, &subject, &disabled)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Principal{}, Session{}, ErrBadCredential
	}
	if err != nil {
		return domain.Principal{}, Session{}, fmt.Errorf("auth: read session: %w", err)
	}

	if revoked != nil {
		s.RevokedAt = *revoked
	}
	switch {
	case !s.RevokedAt.IsZero():
		return domain.Principal{}, Session{}, ErrBadCredential
	case disabled != nil:
		// A disabled account must lose access immediately, not when its
		// session happens to lapse.
		return domain.Principal{}, Session{}, ErrBadCredential
	case !now.Before(s.ExpiresAt):
		return domain.Principal{}, Session{}, ErrExpired
	}

	// Best effort: a failed touch must not fail the request.
	_, _ = p.pool.Exec(ctx,
		`update sessions set last_used_at = $2 where session_id = $1`, s.ID, now)

	grants, err := p.grantsFor(ctx, s.PrincipalID)
	if err != nil {
		return domain.Principal{}, Session{}, err
	}

	return domain.Principal{
		ID:      domain.UserID(s.PrincipalID),
		Subject: subject,
		Display: disp,
		Kind:    domain.PrincipalKind(kind),
		Grants:  grants,
	}, s, nil
}

// PrincipalByToken resolves a long-lived credential for CI, the CLI or a
// webhook sender.
func (p *Postgres) PrincipalByToken(ctx context.Context, tokenHash []byte, now time.Time) (domain.Principal, error) {
	var (
		principalID, kind, disp, subject string
		expires, revoked, disabled       *time.Time
	)

	err := p.pool.QueryRow(ctx, `
		select t.principal_id, t.expires_at, t.revoked_at,
		       pr.kind, pr.display, pr.subject, pr.disabled_at
		from api_tokens t
		join principals pr on pr.principal_id = t.principal_id
		where t.token_hash = $1`, tokenHash,
	).Scan(&principalID, &expires, &revoked, &kind, &disp, &subject, &disabled)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Principal{}, ErrBadCredential
	}
	if err != nil {
		return domain.Principal{}, fmt.Errorf("auth: read token: %w", err)
	}

	switch {
	case revoked != nil, disabled != nil:
		return domain.Principal{}, ErrBadCredential
	case expires != nil && !now.Before(*expires):
		return domain.Principal{}, ErrExpired
	}

	_, _ = p.pool.Exec(ctx,
		`update api_tokens set last_used_at = $2 where token_hash = $1`, tokenHash, now)

	grants, err := p.grantsFor(ctx, principalID)
	if err != nil {
		return domain.Principal{}, err
	}

	return domain.Principal{
		ID:      domain.UserID(principalID),
		Subject: subject,
		Display: disp,
		Kind:    domain.PrincipalKind(kind),
		Grants:  grants,
	}, nil
}

func (p *Postgres) grantsFor(ctx context.Context, principalID string) ([]domain.Grant, error) {
	rows, err := p.pool.Query(ctx, `
		select company_id, area_id, role from role_grants where principal_id = $1`, principalID)
	if err != nil {
		return nil, fmt.Errorf("auth: read grants: %w", err)
	}
	defer rows.Close()

	var out []domain.Grant
	for rows.Next() {
		var company, area, role string
		if err := rows.Scan(&company, &area, &role); err != nil {
			return nil, err
		}
		// A row carrying a role this build does not know is skipped rather
		// than failing the request: a downgrade must not lock everyone out.
		parsed, err := domain.ParseRole(role)
		if err != nil {
			continue
		}
		out = append(out, domain.Grant{
			Scope: domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)},
			Role:  parsed,
		})
	}
	return out, rows.Err()
}

// UpsertPrincipal records whoever just signed in.
//
// Identity is keyed on (provider, subject), never on the email: people change
// their address, and matching on it would hand one person's grants to whoever
// inherits their old mailbox.
func (p *Postgres) UpsertPrincipal(ctx context.Context, provider, subject, display, email string) (string, error) {
	id := "usr_" + shortHash(provider+"|"+subject)

	_, err := p.pool.Exec(ctx, `
		insert into principals (principal_id, kind, provider, subject, display, email, last_seen_at)
		values ($1, 'user', $2, $3, $4, nullif($5,''), now())
		on conflict (provider, subject) where subject <> '' do update set
			display      = excluded.display,
			email        = excluded.email,
			last_seen_at = now()`,
		id, provider, subject, display, email)
	if err != nil {
		return "", fmt.Errorf("auth: upsert principal: %w", err)
	}

	// The conflict target may have matched an existing row with a different
	// generated id; read back the authoritative one.
	var stored string
	if err := p.pool.QueryRow(ctx,
		`select principal_id from principals where provider = $1 and subject = $2`,
		provider, subject).Scan(&stored); err != nil {
		return "", fmt.Errorf("auth: read principal: %w", err)
	}
	return stored, nil
}

// ReplaceGrants sets a principal's grants to exactly what the identity
// provider currently asserts.
//
// Replacement rather than merge: a person removed from a group must lose the
// grant on their next sign-in. Merging would make group membership grant
// access permanently and revoke nothing, which is the failure people discover
// during an audit rather than a review.
// CreateSession issues a browser session and returns the secret to set.
func (p *Postgres) CreateSession(ctx context.Context, principalID, userAgent, ip string, now time.Time) (Token, Session, error) {
	token, err := NewToken()
	if err != nil {
		return Token{}, Session{}, err
	}

	s := Session{
		ID:          "ses_" + shortHash(principalID+now.String()),
		PrincipalID: principalID,
		CreatedAt:   now,
		ExpiresAt:   now.Add(SessionTTL),
	}

	if _, err := p.pool.Exec(ctx, `
		insert into sessions (session_id, principal_id, token_hash, user_agent, ip, created_at, last_used_at, expires_at)
		values ($1, $2, $3, $4, $5, $6, $6, $7)`,
		s.ID, s.PrincipalID, token.Hash, userAgent, ip, s.CreatedAt, s.ExpiresAt); err != nil {
		return Token{}, Session{}, fmt.Errorf("auth: create session: %w", err)
	}
	return token, s, nil
}

// RevokeSession signs one session out.
func (p *Postgres) RevokeSession(ctx context.Context, tokenHash []byte, now time.Time) error {
	_, err := p.pool.Exec(ctx,
		`update sessions set revoked_at = $2 where token_hash = $1 and revoked_at is null`,
		tokenHash, now)
	if err != nil {
		return fmt.Errorf("auth: revoke session: %w", err)
	}
	return nil
}

// RevokeAllSessions signs a principal out everywhere — the action an operator
// takes when a laptop goes missing.
func (p *Postgres) RevokeAllSessions(ctx context.Context, principalID string, now time.Time) error {
	_, err := p.pool.Exec(ctx,
		`update sessions set revoked_at = $2 where principal_id = $1 and revoked_at is null`,
		principalID, now)
	if err != nil {
		return fmt.Errorf("auth: revoke sessions: %w", err)
	}
	return nil
}

// IssueToken mints a long-lived credential. The secret is returned once and
// never recoverable — the database holds only its hash.
func (p *Postgres) IssueToken(ctx context.Context, principalID, name, createdBy string, expires *time.Time) (Token, error) {
	token, err := NewToken()
	if err != nil {
		return Token{}, err
	}

	if _, err := p.pool.Exec(ctx, `
		insert into api_tokens (token_id, principal_id, name, token_hash, created_by, expires_at)
		values ($1, $2, $3, $4, $5, $6)`,
		"tok_"+shortHash(principalID+name), principalID, name, token.Hash, createdBy, expires); err != nil {
		return Token{}, fmt.Errorf("auth: issue token: %w", err)
	}
	return token, nil
}

// PurgeExpired removes lapsed sessions. Housekeeping, not security — an
// expired session is already refused on read.
func (p *Postgres) PurgeExpired(ctx context.Context, before time.Time) (int64, error) {
	tag, err := p.pool.Exec(ctx, `delete from sessions where expires_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("auth: purge sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}
