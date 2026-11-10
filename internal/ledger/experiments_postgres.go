package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
Experiments are an agent's experiments, newest first by the moment each opened, at most
limit of them (NT-015 § Experiments), each with how many runs and cases it opened and the
opening step of its first run.

Derived from the runs, as batteries are: an experiment is exactly the simulated runs whose
opening step names it. A battery names none, and neither does a real run.

Two queries for a whole page, whatever the experiments hold: one that groups the runs and
picks each experiment's first, and one that reads those openings together. Read run by run,
a page of twenty experiments of two hundred runs each was thousands of round trips.
*/
func (p *Postgres) Experiments(
	ctx context.Context, agent domain.AgentID, limit int,
) ([]domain.ExperimentListing, error) {
	if limit <= 0 {
		return nil, nil
	}
	out, firsts, err := p.experimentGroups(ctx, agent, limit)
	if err != nil || len(out) == 0 {
		return nil, err
	}
	rows, err := p.pool.Query(ctx, `
		select `+stepColumns+` from run_steps
		where run_id = any($1) and seq = 1`, firsts)
	if err != nil {
		return nil, fmt.Errorf("openings of the experiments of %s: %w", agent, err)
	}
	defer rows.Close()
	openings, err := scanSteps(rows)
	if err != nil {
		return nil, fmt.Errorf("scan experiment opening: %w", err)
	}
	byRun := make(map[domain.RunID]domain.Step, len(openings))
	for _, st := range openings {
		byRun[st.RunID] = st
	}
	for i := range out {
		out[i].Opening = byRun[domain.RunID(firsts[i])]
	}
	return out, nil
}

// experimentGroups is a page of an agent's experiments with their counts, and the first
// run of each, in the same order.
//
// A judge run names the experiment it judges and is not one of its runs (NT-016), so it
// counts in no group and is never a group's first.
func (p *Postgres) experimentGroups(
	ctx context.Context, agent domain.AgentID, limit int,
) ([]domain.ExperimentListing, []string, error) {
	rows, err := p.pool.Query(ctx, `
		select s.payload->>'experiment' as experiment, min(r.started_at) as opened,
		       count(*) as runs, count(distinct coalesce(s.payload->>'case', '')) as cases,
		       (array_agg(r.run_id order by r.started_at, r.run_id))[1] as first_run
		from runs r
		join run_steps s on s.run_id = r.run_id and s.seq = 1 and s.kind = 'run_started'
		where r.agent_id = $1 and r.simulated
		  and coalesce(s.payload->>'experiment', '') <> ''
		  and coalesce(jsonb_typeof(s.payload->'judge'), '') <> 'object'
		group by experiment
		order by opened desc, experiment desc
		limit $2`, string(agent), limit)
	if err != nil {
		return nil, nil, fmt.Errorf("experiments of %s: %w", agent, err)
	}
	defer rows.Close()

	var out []domain.ExperimentListing
	var firsts []string
	for rows.Next() {
		var e domain.ExperimentListing
		var opened time.Time
		var first string
		if err := rows.Scan(&e.ID, &opened, &e.Runs, &e.Cases, &first); err != nil {
			return nil, nil, fmt.Errorf("scan experiment: %w", err)
		}
		e.OpenedAt = opened
		out, firsts = append(out, e), append(firsts, first)
	}
	return out, firsts, rows.Err()
}
