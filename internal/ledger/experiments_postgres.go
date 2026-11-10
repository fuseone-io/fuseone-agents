package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
Experiments are an agent's experiments, newest first by the moment each opened, at most
limit of them (NT-015 § Experiments).

Derived from the runs, as batteries are: an experiment is exactly the simulated runs whose
opening step names it. A battery names none, and neither does a real run.
*/
func (p *Postgres) Experiments(ctx context.Context, agent domain.AgentID, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := p.pool.Query(ctx, `
		select s.payload->>'experiment' as experiment, min(r.started_at) as opened
		from runs r
		join run_steps s on s.run_id = r.run_id and s.seq = 1 and s.kind = 'run_started'
		where r.agent_id = $1 and r.simulated
		  and coalesce(s.payload->>'experiment', '') <> ''
		group by experiment
		order by opened desc, experiment desc
		limit $2`, string(agent), limit)
	if err != nil {
		return nil, fmt.Errorf("experiments of %s: %w", agent, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var experiment string
		var opened time.Time
		if err := rows.Scan(&experiment, &opened); err != nil {
			return nil, fmt.Errorf("scan experiment: %w", err)
		}
		out = append(out, experiment)
	}
	return out, rows.Err()
}
