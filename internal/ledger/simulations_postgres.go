package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

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

/*
Latest is the most recent simulation run against one version.

Derived from the runs rather than tracked beside them, like everything else
here: a simulation is exactly the set of runs that name it, so the newest one
for a version is a question about those runs and a table recording it could
disagree with them.

Simulated runs only. A real run is not a rehearsal and counting one would let
an agent be started on the strength of having been used.
*/
func (p *Postgres) Latest(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) (string, bool, error) {
	var simulation string
	err := p.pool.QueryRow(ctx, `
		select simulation from runs
		where agent_id = $1 and version_id = $2 and simulated and simulation <> ''
		order by started_at desc
		limit 1`, string(agent), string(version)).Scan(&simulation)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("latest simulation of %s@%s: %w", agent, version, err)
	}
	return simulation, true, nil
}
