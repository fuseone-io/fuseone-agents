package ledger

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// AgentActivity aggregates each agent's runs.
//
// One grouped read over the projection rather than one query per agent: a
// screen listing twenty agents should cost one round trip, not twenty.
func (p *Postgres) AgentActivity(ctx context.Context, filter domain.RunFilter) ([]domain.AgentActivity, error) {
	where, args := runFilterSQL(filter)

	rows, err := p.pool.Query(ctx, `
		select agent_id,
		       count(*),
		       count(*) filter (where phase = 'finished'),
		       count(*) filter (where phase in ('awaiting_approval', 'parked')),
		       coalesce(sum(cost_micros), 0),
		       max(started_at),
		       (array_agg(phase order by started_at desc, run_id desc))[1]
		from runs `+where+`
		group by agent_id order by agent_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("agent activity: %w", err)
	}
	defer rows.Close()

	var out []domain.AgentActivity
	for rows.Next() {
		var (
			a     domain.AgentActivity
			agent string
		)
		if err := rows.Scan(&agent, &a.Runs, &a.Finished, &a.Waiting,
			&a.CostMicros, &a.LastRunAt, &a.LastPhase); err != nil {
			return nil, err
		}
		a.AgentID = domain.AgentID(agent)
		out = append(out, a)
	}
	return out, rows.Err()
}
