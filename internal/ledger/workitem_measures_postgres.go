package ledger

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
WorkItemMeasures reads the weekly measures in one statement, in a read-only
transaction, so the runs, their links and their approvals are one snapshot.

The runs come from the projection by the caller's filter and the window, on
the opening moment the (company, area, started_at) and (agent, started_at)
indexes lead with. Their approval steps are read from run_steps joined on
(run_id, opened_at), with opened_at bounded by the window as parameters: the
months outside it are pruned when the statement is planned, not while it
runs. No list of runs leaves the database or comes back into it.
*/
func (p *Postgres) WorkItemMeasures(
	ctx context.Context, filter domain.RunFilter, cost *domain.RunFilter, window domain.WorkItemWindow,
) ([]domain.WorkItemWeek, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("open the measures' snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	sql, args := workItemMeasuresSQL(filter, window)
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("read the work item measures: %w", err)
	}
	shares, err := scanMeasureShares(rows)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("close the measures' snapshot: %w", err)
	}
	return measuresFrom(shares, cost), nil
}

func scanMeasureShares(rows pgx.Rows) ([]domain.WorkItemWeekShare, error) {
	defer rows.Close()
	out := []domain.WorkItemWeekShare{}
	for rows.Next() {
		var s domain.WorkItemWeekShare
		var cards []string
		p := &s.Proposals
		if err := rows.Scan(&s.Week, &s.AgentID, &s.Scope.Company, &s.Scope.Area, &s.Runs, &s.OpenRuns, &cards,
			&p.Requested, &p.Approved, &p.Refused, &p.Abandoned, &p.Pending,
			&p.RevisedAfterRefusal, &p.RepeatedAfterRefusal, &p.Standing,
			&s.Cost.OnCardsMicros, &s.Cost.InheritedMicros, &s.Cost.OffCardMicros); err != nil {
			return nil, fmt.Errorf("scan a week's measures: %w", err)
		}
		s.Week = s.Week.UTC()
		for _, c := range cards {
			s.Cards = append(s.Cards, domain.WorkItemRef(c))
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read a week's measures: %w", err)
	}
	return out, nil
}

/*
workItemMeasuresSQL is the measures' one statement.

  - r: the runs the filter reads, real, opened inside the window, each with
    its link — a claim (of a card or a pull request, resolved through a
    readable alias in the run's own company to its root), else an
    inheritance, else none — and its ISO week in UTC.
  - asked: the window's approval steps of those runs, by (run_id, opened_at),
    carrying the verdict, the request a decision answers, the tool and the
    digest of the arguments: never the arguments, a reason or a note.
  - refusals: each refusal that names its request, with whether the same run
    later proposed the same tool with other arguments, or with the same ones.
*/
func workItemMeasuresSQL(filter domain.RunFilter, window domain.WorkItemWindow) (string, []any) {
	where, args := runFilterSQL(measuresFilter(filter, window))
	where = whereAnd(where, realRuns)
	readable, args := aliasScopeSQL(aliasReader(filter), args)
	args = append(args, window.From.UTC(), window.To.UTC())
	from, to := len(args)-1, len(args)
	return `with ` + fmt.Sprintf(measuresRunsSQL, where, readable) + `,
		` + fmt.Sprintf(measuresAskedSQL, from, to) + `,
		` + measuresRefusalsSQL + measuresRowsSQL, args
}

// measuresRunsSQL is r: the runs, given the filter's WHERE and the
// readable-alias predicate.
//
// Each link is a subquery per run, by the primary keys of run_work_items and
// run_work_item_origins and the (company_id, ref) key of work_item_aliases,
// rather than a join: joined, the planner could hash the whole of a link
// table — every claim the ledger ever recorded — to match a window's runs.
// x is materialized so each subquery runs once per run, not once for every
// place r reads its result. The alias predicate's bare company_id and
// area_id are the alias row's own.
const measuresRunsSQL = `x as materialized (
			select run_id, agent_id, company_id, area_id, started_at, cost_micros,
			       phase not in ('finished', 'failed') as open, pending_at_seq is not null as pending,
			       (select w.ref from run_work_items w where w.run_id = runs.run_id) as claimed,
			       (select o.source from run_work_item_origins o where o.run_id = runs.run_id) is not null as inherited
			from runs %s
		),
		r as (
			select x.run_id, x.agent_id, x.company_id, x.area_id, x.started_at, x.cost_micros, x.open, x.pending,
			       date_trunc('week', x.started_at, 'UTC') as week,
			       case when x.claimed is not null then 'claimed'
			            when x.inherited then 'inherited' else 'none' end as link,
			       case when x.claimed is not null then coalesce(
			           (select a.root_ref from work_item_aliases a
			            where a.company_id = x.company_id and a.ref = x.claimed and %s),
			           x.claimed) end as card
			from x
		)`

// measuresAskedSQL is asked and per_run, given the parameters that hold the
// window's bounds. A standing approval is one naming its grant
// (domain.IsStandingDecision), never one whose note reads like it.
const measuresAskedSQL = `asked as (
			select s.run_id, s.seq, s.kind,
			       coalesce(s.payload->>'tool', '') as tool,
			       coalesce(s.payload->>'args_digest', '') as digest,
			       coalesce((s.payload->>'approved')::boolean, false) as approved,
			       (s.payload->>'at_seq')::bigint as at_seq,
			       coalesce(s.payload->>'standing_grant', '') <> '' as standing
			from run_steps s
			join r on r.run_id = s.run_id and r.started_at = s.opened_at
			where s.opened_at >= $%[1]d and s.opened_at < $%[2]d
			  and s.kind in ('approval_requested', 'approval_decided', 'abandoned')
		),
		per_run as (
			select run_id,
			       count(*) filter (where kind = 'approval_requested') as requested,
			       count(*) filter (where kind = 'approval_decided' and approved) as approved,
			       count(*) filter (where kind = 'approval_decided' and not approved) as refused,
			       count(*) filter (where kind = 'abandoned') as abandoned,
			       count(*) filter (where kind = 'approval_decided' and approved and standing) as standing
			from asked group by run_id
		)`

// measuresRefusalsSQL is refusals: per run, the refusals naming their
// request that the same run followed with the same tool, by digest.
const measuresRefusalsSQL = `refusals as (
			select d.run_id,
			       count(*) filter (where exists (
			           select 1 from asked n where n.run_id = d.run_id and n.kind = 'approval_requested'
			             and n.seq > d.seq and n.tool = q.tool and n.digest <> q.digest)) as revised,
			       count(*) filter (where exists (
			           select 1 from asked n where n.run_id = d.run_id and n.kind = 'approval_requested'
			             and n.seq > d.seq and n.tool = q.tool and n.digest = q.digest)) as repeated
			from asked d
			join asked q on q.run_id = d.run_id and q.seq = d.at_seq and q.kind = 'approval_requested'
			where d.kind = 'approval_decided' and not d.approved and d.at_seq is not null
			group by d.run_id
		)`

// measuresRowsSQL is the shares: one row per week, agent and scope.
const measuresRowsSQL = `
		select r.week, r.agent_id, r.company_id, r.area_id, count(*)::int, count(*) filter (where r.open)::int,
		       coalesce(array_agg(distinct r.card) filter (where r.card is not null), '{}'),
		       coalesce(sum(p.requested), 0)::int, coalesce(sum(p.approved), 0)::int,
		       coalesce(sum(p.refused), 0)::int, coalesce(sum(p.abandoned), 0)::int,
		       count(*) filter (where r.pending)::int,
		       coalesce(sum(f.revised), 0)::int, coalesce(sum(f.repeated), 0)::int,
		       coalesce(sum(p.standing), 0)::int,
		       coalesce(sum(r.cost_micros) filter (where r.link = 'claimed'), 0)::bigint,
		       coalesce(sum(r.cost_micros) filter (where r.link = 'inherited'), 0)::bigint,
		       coalesce(sum(r.cost_micros) filter (where r.link = 'none'), 0)::bigint
		from r
		left join per_run p on p.run_id = r.run_id
		left join refusals f on f.run_id = r.run_id
		group by r.week, r.agent_id, r.company_id, r.area_id`
