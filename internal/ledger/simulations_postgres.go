package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

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
Batteries are the simulations run against one version, newest first.

Derived from the runs rather than tracked beside them, like everything else
here: a simulation is exactly the set of runs that name it, and a table
recording it could disagree with them.

Simulated runs only. A real run is not a rehearsal and counting one would let
an agent be started on the strength of having been used.
*/
func (p *Postgres) Batteries(
	ctx context.Context, agent domain.AgentID, version domain.VersionID, limit int,
) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := p.pool.Query(ctx, `
		select simulation, max(started_at) as opened from runs
		where agent_id = $1 and version_id = $2 and simulated and simulation <> ''
		group by simulation
		order by opened desc
		limit $3`, string(agent), string(version), limit)
	if err != nil {
		return nil, fmt.Errorf("batteries of %s@%s: %w", agent, version, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var simulation string
		var opened time.Time
		if err := rows.Scan(&simulation, &opened); err != nil {
			return nil, fmt.Errorf("scan battery: %w", err)
		}
		out = append(out, simulation)
	}
	return out, rows.Err()
}

// Latest is the newest battery run against one version.
func (p *Postgres) Latest(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) (string, bool, error) {
	found, err := p.Batteries(ctx, agent, version, 1)
	if err != nil || len(found) == 0 {
		return "", false, err
	}
	return found[0], true, nil
}

// LastBatteryAt is when a version's corpus last ran, which is what decides
// whether it runs again tonight.
//
// The moment the newest battery opened, not the moment it finished: a battery
// still being advanced is one that has already been paid for, and a clock
// reading the finish would open a second one beside it every pass.
func (p *Postgres) LastBatteryAt(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) (time.Time, bool, error) {
	// Nullable, because max() over no rows is NULL rather than no row — and
	// scanning that into a time.Time is an error that reads like a broken
	// query when it means "this corpus has never run".
	var at *time.Time
	err := p.pool.QueryRow(ctx, `
		select max(started_at) from runs
		where agent_id = $1 and version_id = $2 and simulated and simulation <> ''`,
		string(agent), string(version)).Scan(&at)

	if errors.Is(err, pgx.ErrNoRows) || (err == nil && at == nil) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf(
			"last battery of %s@%s: %w", agent, version, err)
	}
	return *at, true, nil
}
