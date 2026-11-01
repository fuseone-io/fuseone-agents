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
// the ability to make agents run. A GitHub hook keeps a sealed copy beside the
// hash, because GitHub signs with the secret instead of sending it — see
// webhooks_github_postgres.go.
type PostgresWebhooks struct {
	pool  *pgxpool.Pool
	vault Sealer
}

func NewPostgresWebhooks(pool *pgxpool.Pool) *PostgresWebhooks {
	return &PostgresWebhooks{pool: pool}
}

// hookColumns is what Find and ForAgent read, in scanHook's order.
const hookColumns = `path, agent_id, company_id, area_id, secret_hash, rotated_by, rotated_at,
	source, events, secret_sealed`

// scanHook reads one row of hookColumns.
//
// A GitHub hook is armed only with its sealed copy: a hash alone cannot check
// a signature, so a path that has one and not the other is closed.
func scanHook(row pgx.Row) (Hook, error) {
	var (
		hook          Hook
		company, area string
		secret        []byte
		sealed        []byte
		rotated       *time.Time
	)
	if err := row.Scan(&hook.Path, &hook.Agent, &company, &area, &secret, &hook.By, &rotated,
		&hook.From, &hook.Events, &sealed); err != nil {
		return Hook{}, err
	}
	hook.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	hook.Armed = len(secret) > 0 && (hook.From != domain.WebhookFromGitHub || len(sealed) > 0)
	if len(hook.Events) == 0 {
		hook.Events = nil
	}
	if rotated != nil {
		hook.Rotated = rotated.UTC()
	}
	return hook, nil
}

// Find returns what is declared at a path.
func (p *PostgresWebhooks) Find(ctx context.Context, path string) (Hook, error) {
	hook, err := scanHook(p.pool.QueryRow(ctx,
		`select `+hookColumns+` from webhook_triggers where path = $1`, path))
	if errors.Is(err, pgx.ErrNoRows) {
		return Hook{}, ErrNoHook
	}
	if err != nil {
		return Hook{}, fmt.Errorf("trigger: find hook %q: %w", path, err)
	}
	return hook, nil
}

// Verify reports whether the offered secret is the one stored for the path.
//
// Never for a GitHub hook: its secret signs deliveries, and accepting it as a
// sent secret would let whoever holds it skip the signature.
func (p *PostgresWebhooks) Verify(ctx context.Context, path, secret string) (bool, error) {
	var (
		stored []byte
		source string
	)
	err := p.pool.QueryRow(ctx,
		`select secret_hash, source from webhook_triggers where path = $1`, path).Scan(&stored, &source)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNoHook
	}
	if err != nil {
		return false, fmt.Errorf("trigger: verify %q: %w", path, err)
	}
	if len(stored) == 0 {
		return false, ErrNotArmed
	}
	if source != "" {
		return false, nil
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
	var source string
	err := p.pool.QueryRow(ctx, `select source from webhook_triggers where path = $1`, path).Scan(&source)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoHook
	}
	if err != nil {
		return "", fmt.Errorf("trigger: rotate %q: %w", path, err)
	}

	secret, hash, err := NewSecret()
	if err != nil {
		return "", err
	}
	sealed, nonce, err := p.sealFor(source, path, secret)
	if err != nil {
		return "", err
	}

	tag, err := p.pool.Exec(ctx, `
		update webhook_triggers
		set secret_hash = $2, rotated_by = $3, rotated_at = $4,
			secret_sealed = $5, secret_nonce = $6
		where path = $1 and source = $7`, path, hash, string(by), at.UTC(), sealed, nonce, source)
	if err != nil {
		return "", fmt.Errorf("trigger: rotate %q: %w", path, err)
	}
	if tag.RowsAffected() == 0 {
		// Gone, or republished with another source between the read and the
		// write: either way this secret was sealed for a hook that no longer
		// exists as read.
		return "", ErrNoHook
	}
	return secret, nil
}

// Sync reconciles an agent's declared webhooks with what its newest version
// says.
//
// A path that is still declared keeps its secret: rewriting it on every
// publish would break every sender the moment somebody edited a prompt —
// unless its source changed, because a secret handed to a sender who sends it
// must not start working for one who signs with it, or the reverse. A path no
// longer declared is deleted, because a version that withdrew a webhook must
// stop answering on it.
func (p *PostgresWebhooks) Sync(
	ctx context.Context, agent domain.AgentID, scope domain.Scope, hooks []domain.WebhookTrigger,
) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("trigger: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	paths := []string{}
	for _, hook := range hooks {
		if err := claimHook(ctx, tx, agent, scope, hook); err != nil {
			return err
		}
		paths = append(paths, hook.Path)
	}

	// Never nil: nil binds as NULL and the delete would match nothing,
	// leaving a withdrawn webhook answering forever.
	if _, err := tx.Exec(ctx, `
		delete from webhook_triggers
		where agent_id = $1 and path <> all($2::text[])`,
		string(agent), paths); err != nil {
		return fmt.Errorf("trigger: prune hooks of %s: %w", agent, err)
	}
	return tx.Commit(ctx)
}

// claimHook writes one declared webhook.
//
// Claimed, never taken. A path already answering for somebody else has a
// sender holding its secret, and quietly repointing that path would aim an
// existing key at a different agent.
func claimHook(
	ctx context.Context, tx pgx.Tx, agent domain.AgentID, scope domain.Scope, hook domain.WebhookTrigger,
) error {
	events := hook.Events
	if events == nil {
		events = []string{}
	}
	var owner string
	err := tx.QueryRow(ctx, `
		insert into webhook_triggers (path, agent_id, company_id, area_id, source, events)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (path) do update set
			company_id = excluded.company_id,
			area_id = excluded.area_id,
			events = excluded.events,
			source = excluded.source,
			secret_hash = case when webhook_triggers.source = excluded.source
				then webhook_triggers.secret_hash end,
			secret_sealed = case when webhook_triggers.source = excluded.source
				then webhook_triggers.secret_sealed end,
			secret_nonce = case when webhook_triggers.source = excluded.source
				then webhook_triggers.secret_nonce end
		where webhook_triggers.agent_id = excluded.agent_id
		returning agent_id`,
		hook.Path, string(agent), string(scope.Company), string(scope.Area), hook.From, events,
	).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %q belongs to another agent", ErrPathTaken, hook.Path)
	}
	if err != nil {
		return fmt.Errorf("trigger: sync hook %q: %w", hook.Path, err)
	}
	return nil
}

// ForAgent lists what an agent declares, for the console.
func (p *PostgresWebhooks) ForAgent(ctx context.Context, agent domain.AgentID) ([]Hook, error) {
	rows, err := p.pool.Query(ctx,
		`select `+hookColumns+` from webhook_triggers where agent_id = $1 order by path`, string(agent))
	if err != nil {
		return nil, fmt.Errorf("trigger: hooks of %s: %w", agent, err)
	}
	defer rows.Close()

	var out []Hook
	for rows.Next() {
		hook, err := scanHook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, hook)
	}
	return out, rows.Err()
}
