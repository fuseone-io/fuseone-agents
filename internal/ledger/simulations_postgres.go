package ledger

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// SimulationRuns lists the runs one simulation opened, in the order its cases
// were run.
//
// An empty id matches nothing. Real runs carry no simulation, and a query that
// treated "" as "everything unmarked" would report production as a simulation.
func (p *Postgres) SimulationRuns(ctx context.Context, simulation string) ([]domain.RunID, error) {
	if simulation == "" {
		return nil, nil
	}

	rows, err := p.pool.Query(ctx, `
		select run_id from runs
		where simulation = $1
		order by started_at, run_id`, simulation)
	if err != nil {
		return nil, fmt.Errorf("list simulation runs: %w", err)
	}
	defer rows.Close()

	var out []domain.RunID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan simulation run: %w", err)
		}
		out = append(out, domain.RunID(id))
	}
	return out, rows.Err()
}

/*
HasSimulation reports whether an agent has ever been run against occurrences
that already happened.

It is the gate on leaving Draft (PRD FU-10). What it checks is that a
simulation exists — not that anybody read it. Reviewing is the half of that
requirement this platform cannot observe: it can put the report in front of a
person and record that somebody asked for it, and it cannot know they thought
about it. Claiming otherwise in a check would be worse than the gap.
*/
func (p *Postgres) HasSimulation(ctx context.Context, agent domain.AgentID) (bool, error) {
	var exists bool
	err := p.pool.QueryRow(ctx, `
		select exists (
			select 1 from runs where agent_id = $1 and simulated
		)`, string(agent)).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("ledger: read simulations of %s: %w", agent, err)
	}
	return exists, nil
}
