package trigger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// PostgresWebhooks keeps the declared paths and their secrets.
//
// The secret is stored only as a hash: a database dump must not hand somebody
// the ability to make agents run.
type PostgresWebhooks struct{ pool *pgxpool.Pool }

func NewPostgresWebhooks(pool *pgxpool.Pool) *PostgresWebhooks {
	return &PostgresWebhooks{pool: pool}
}

// Find returns what is declared at a path.
func (p *PostgresWebhooks) Find(ctx context.Context, path string) (Hook, error) {
	var (
		hook    Hook
		company string
		area    string
		secret  []byte
		rotated *time.Time
	)
	err := p.pool.QueryRow(ctx, `
		select path, agent_id, company_id, area_id, secret_hash, rotated_by, rotated_at
		from webhook_triggers where path = $1`, path,
	).Scan(&hook.Path, &hook.Agent, &company, &area, &secret, &hook.By, &rotated)

	if errors.Is(err, pgx.ErrNoRows) {
		return Hook{}, ErrNoHook
	}
	if err != nil {
		return Hook{}, fmt.Errorf("trigger: find hook %q: %w", path, err)
	}

	hook.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	hook.Armed = len(secret) > 0
	if rotated != nil {
		hook.Rotated = rotated.UTC()
	}
	return hook, nil
}

// Verify reports whether the offered secret is the one stored for the path.
func (p *PostgresWebhooks) Verify(ctx context.Context, path, secret string) (bool, error) {
	var stored []byte
	err := p.pool.QueryRow(ctx,
		`select secret_hash from webhook_triggers where path = $1`, path).Scan(&stored)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNoHook
	}
	if err != nil {
		return false, fmt.Errorf("trigger: verify %q: %w", path, err)
	}
	if len(stored) == 0 {
		return false, ErrNotArmed
	}
	return MatchesSecret(stored, secret), nil
}

// Rotate replaces the secret and returns the new one, once.
//
// Rotating is also how the first one is generated: there is no separate
// "arm" — a hook either has a current secret or it is closed.
func (p *PostgresWebhooks) Rotate(
	ctx context.Context, path string, by domain.UserID, at time.Time,
) (string, error) {
	secret, hash, err := NewSecret()
	if err != nil {
		return "", err
	}

	tag, err := p.pool.Exec(ctx, `
		update webhook_triggers
		set secret_hash = $2, rotated_by = $3, rotated_at = $4
		where path = $1`, path, hash, string(by), at.UTC())
	if err != nil {
		return "", fmt.Errorf("trigger: rotate %q: %w", path, err)
	}
	if tag.RowsAffected() == 0 {
		return "", ErrNoHook
	}
	return secret, nil
}

// Sync reconciles an agent's declared paths with what its newest version says.
//
// A path that is still declared keeps its secret: rewriting it on every
// publish would break every sender the moment somebody edited a prompt. A path
// no longer declared is deleted, because a version that withdrew a webhook must
// stop answering on it.
func (p *PostgresWebhooks) Sync(
	ctx context.Context, agent domain.AgentID, scope domain.Scope, paths []string,
) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("trigger: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, path := range paths {
		// Claimed, never taken. A path already answering for somebody else has
		// a sender holding its secret, and quietly repointing that path would
		// aim an existing key at a different agent.
		var owner string
		if err := tx.QueryRow(ctx, `
			insert into webhook_triggers (path, agent_id, company_id, area_id)
			values ($1, $2, $3, $4)
			on conflict (path) do update set
				company_id = excluded.company_id,
				area_id = excluded.area_id
			where webhook_triggers.agent_id = excluded.agent_id
			returning agent_id`,
			path, string(agent), string(scope.Company), string(scope.Area),
		).Scan(&owner); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: %q belongs to another agent", ErrPathTaken, path)
			}
			return fmt.Errorf("trigger: sync hook %q: %w", path, err)
		}
	}

	// Empty rather than nil: nil binds as NULL and the delete would match
	// nothing, leaving a withdrawn webhook answering forever.
	if paths == nil {
		paths = []string{}
	}
	if _, err := tx.Exec(ctx, `
		delete from webhook_triggers
		where agent_id = $1 and path <> all($2::text[])`,
		string(agent), paths); err != nil {
		return fmt.Errorf("trigger: prune hooks of %s: %w", agent, err)
	}
	return tx.Commit(ctx)
}

// ForAgent lists what an agent declares, for the console.
func (p *PostgresWebhooks) ForAgent(ctx context.Context, agent domain.AgentID) ([]Hook, error) {
	rows, err := p.pool.Query(ctx, `
		select path, agent_id, company_id, area_id, secret_hash, rotated_by, rotated_at
		from webhook_triggers where agent_id = $1 order by path`, string(agent))
	if err != nil {
		return nil, fmt.Errorf("trigger: hooks of %s: %w", agent, err)
	}
	defer rows.Close()

	var out []Hook
	for rows.Next() {
		var (
			hook          Hook
			company, area string
			secret        []byte
			rotated       *time.Time
		)
		if err := rows.Scan(&hook.Path, &hook.Agent, &company, &area,
			&secret, &hook.By, &rotated); err != nil {
			return nil, err
		}
		hook.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		hook.Armed = len(secret) > 0
		if rotated != nil {
			hook.Rotated = rotated.UTC()
		}
		out = append(out, hook)
	}
	return out, rows.Err()
}
