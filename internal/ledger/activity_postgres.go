package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// AgentActivity aggregates each agent's runs.
//
// One grouped read over the projection rather than one query per agent: a
// screen listing twenty agents should cost one round trip, not twenty.
func (p *Postgres) AgentActivity(ctx context.Context, filter domain.RunFilter) ([]domain.AgentActivity, error) {
	where, args := runFilterSQL(filter)
	where = whereAnd(where, realRuns)

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

// SpentSince is what a scope consumed in the window a ceiling covers.
//
// Summed in the database, and deliberately not derived from CostRollup: that
// answers "how was the money divided", this answers "how much is left", and
// tying them would make a ceiling depend on how somebody chose to group a
// report.
func (p *Postgres) SpentSince(ctx context.Context, scope domain.Scope, since time.Time) (domain.Consumption, error) {
	where, args := runFilterSQL(domain.RunFilter{Scope: scope, Since: since})
	where = whereAnd(where, realRuns)

	var c domain.Consumption
	err := p.pool.QueryRow(ctx, `
		select coalesce(sum(cost_micros), 0),
		       coalesce(sum(total_tokens), 0),
		       coalesce(sum(tool_calls), 0),
		       coalesce(sum(last_seq), 0)
		from runs `+where, args...,
	).Scan(&c.Micros, &c.Tokens, &c.ToolCalls, &c.Steps)
	if err != nil {
		return domain.Consumption{}, fmt.Errorf("spend since: %w", err)
	}
	return c, nil
}

/*
Agreement counts what people decided about each agent's proposals.

Read from the trail rather than from a counter, for the reason every other
number here is: a counter drifts and the trail is what happened. It counts
approval decisions only — a run nobody was asked about says nothing about
whether they would have agreed, and reading silence as consent is how an agent
gets promoted for never having been checked.

Simulated runs are excluded. Nobody was really asked in one.
*/
func (p *Postgres) Agreement(ctx context.Context, since time.Time) ([]domain.Agreement, error) {
	rows, err := p.pool.Query(ctx, `
		select agent_id,
		       count(*) filter (where (payload->>'approved')::boolean),
		       count(*) filter (where not (payload->>'approved')::boolean)
		from run_steps
		where kind = 'approval_decided'
		  and at >= $1
		  and `+realSteps+`
		group by agent_id
		order by agent_id`, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("agreement: %w", err)
	}
	defer rows.Close()

	var out []domain.Agreement
	for rows.Next() {
		var a domain.Agreement
		var agent string
		if err := rows.Scan(&agent, &a.Approved, &a.Refused); err != nil {
			return nil, fmt.Errorf("agreement: scan: %w", err)
		}
		a.Agent = domain.AgentID(agent)
		out = append(out, a)
	}
	return out, rows.Err()
}

// VersionAgreement counts approval decisions per version.
//
// Same trail as Agreement, narrower question. Version belongs in the group
// because the Trust Center compares published definitions: agreement with v1
// is not evidence about v2.
func (p *Postgres) VersionAgreement(
	ctx context.Context, filter domain.RunFilter,
) ([]domain.VersionAgreement, error) {
	where, args := runFilterOn(filter, "at")
	where = whereAnd(whereAnd(where, "kind = 'approval_decided'"), realSteps)

	rows, err := p.pool.Query(ctx, `
		select agent_id,
		       version_id,
		       count(*) filter (where (payload->>'approved')::boolean),
		       count(*) filter (where not (payload->>'approved')::boolean)
		from run_steps `+where+`
		group by agent_id, version_id
		order by agent_id, version_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("version agreement: %w", err)
	}
	defer rows.Close()

	var out []domain.VersionAgreement
	for rows.Next() {
		var (
			agent, version string
			approved       int64
			refused        int64
		)
		if err := rows.Scan(&agent, &version, &approved, &refused); err != nil {
			return nil, fmt.Errorf("version agreement: scan: %w", err)
		}
		out = append(out, domain.VersionAgreement{
			Agent: domain.AgentID(agent), Version: domain.VersionID(version),
			Approved: int(approved), Refused: int(refused),
		})
	}
	return out, rows.Err()
}

// VersionGateBlocks counts Gate refusals per version.
//
// The Trust Center compares published definitions, so the version is part of
// the bucket. Simulations are excluded: a blocked rehearsal is useful in the
// simulation report, not evidence that production runs are stopping.
func (p *Postgres) VersionGateBlocks(
	ctx context.Context, filter domain.RunFilter,
) ([]domain.VersionGateBlocks, error) {
	where, args := runFilterOn(filter, "at")
	args = append(args, int(domain.VerdictBlock))
	where = whereAnd(whereAnd(where, "kind = 'gate_decided'"),
		fmt.Sprintf("coalesce((payload->>'verdict')::int, 0) = $%d", len(args)))
	where = whereAnd(where, realSteps)

	rows, err := p.pool.Query(ctx, `
		select agent_id, version_id, count(*), count(distinct run_id)
		from run_steps `+where+`
		group by agent_id, version_id
		order by agent_id, version_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("version gate blocks: %w", err)
	}
	defer rows.Close()

	var out []domain.VersionGateBlocks
	for rows.Next() {
		var agent, version string
		var blocks, runs int64
		if err := rows.Scan(&agent, &version, &blocks, &runs); err != nil {
			return nil, fmt.Errorf("version gate blocks: scan: %w", err)
		}
		out = append(out, domain.VersionGateBlocks{
			Agent: domain.AgentID(agent), Version: domain.VersionID(version),
			Blocks: int(blocks), Runs: int(runs),
		})
	}
	return out, rows.Err()
}
