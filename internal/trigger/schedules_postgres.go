package trigger

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// Postgres keeps when each schedule next comes due.
//
// It is not a lock and does not pretend to be one: two workers reading the
// same due row both open the same run, because the key is derived from the
// moment rather than from whoever got here first.
type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

func (p *Postgres) Due(ctx context.Context, at time.Time) ([]Due, error) {
	rows, err := p.pool.Query(ctx, `
		select agent_id, schedule, next_fire_at
		from trigger_schedules
		where next_fire_at <= $1
		order by next_fire_at`, at.UTC())
	if err != nil {
		return nil, fmt.Errorf("trigger: due schedules: %w", err)
	}
	defer rows.Close()

	var out []Due
	for rows.Next() {
		var d Due
		if err := rows.Scan(&d.Agent, &d.Schedule, &d.At); err != nil {
			return nil, err
		}
		d.At = d.At.UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}

func (p *Postgres) Advance(
	ctx context.Context, agent domain.AgentID, schedule string, next time.Time,
) error {
	_, err := p.pool.Exec(ctx, `
		update trigger_schedules
		set next_fire_at = $3, updated_at = now()
		where agent_id = $1 and schedule = $2`,
		string(agent), schedule, next.UTC())
	if err != nil {
		return fmt.Errorf("trigger: advance %s: %w", agent, err)
	}
	return nil
}

// Sync reconciles an agent's schedules with what its newest version declares.
//
// A schedule that is still declared keeps its next moment: rewriting it on
// every publish would let an agent published every few minutes never reach a
// moment at all. One that is gone is deleted, because a version that no longer
// declares a schedule must stop firing it.
func (p *Postgres) Sync(
	ctx context.Context, agent domain.AgentID, schedules []string, from time.Time,
) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("trigger: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, schedule := range schedules {
		next, err := NextAfter(schedule, from)
		if err != nil {
			// Refused here rather than every tick: an unparseable schedule
			// that reached the table would be read as due forever.
			return err
		}
		if _, err := tx.Exec(ctx, `
			insert into trigger_schedules (agent_id, schedule, next_fire_at)
			values ($1, $2, $3)
			on conflict (agent_id, schedule) do nothing`,
			string(agent), schedule, next); err != nil {
			return fmt.Errorf("trigger: sync %s: %w", agent, err)
		}
	}

	// An empty slice, never nil: nil binds as NULL, and `schedule <> all(NULL)`
	// is NULL rather than true — so an agent that withdrew every schedule
	// would quietly keep firing all of them.
	if schedules == nil {
		schedules = []string{}
	}
	if _, err := tx.Exec(ctx, `
		delete from trigger_schedules
		where agent_id = $1 and schedule <> all($2::text[])`,
		string(agent), schedules); err != nil {
		return fmt.Errorf("trigger: prune %s: %w", agent, err)
	}
	return tx.Commit(ctx)
}
