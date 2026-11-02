package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
WorkItemTrail reads a card's trail in at most three statements, none of
which grows with the card's history.

Every statement starts from the same source: run_work_items by its (ref)
index, joined to the runs projection narrowed by the caller's own filter
and stripped of simulations. The filter is the generic one, applied to runs
in a subquery so its columns stay those of runs alone; the card is matched
here, by an explicit predicate, never by teaching the generic filter about
work items.

A page is one statement, limited to the page plus one row. The first page
adds the role summary: the card's earliest and latest opening moments, then
one aggregate grouped by agent and scope that counts the approval steps in
SQL. The two moments bound run_steps' partition key as parameters, so the
summary touches only the months the card's runs opened in, and no list of
runs ever leaves the database or comes back into it.
*/
func (p *Postgres) WorkItemTrail(
	ctx context.Context, filter domain.RunFilter, cost *domain.RunFilter,
	ref domain.WorkItemRef, page domain.WorkItemPage,
) (domain.WorkItemTrail, error) {
	runs, err := p.workItemRuns(ctx, filter, ref, page.After, page.Size()+1)
	if err != nil {
		return domain.WorkItemTrail{}, err
	}
	var shares []domain.WorkItemRoleShare
	if page.After == nil {
		if shares, err = p.workItemShares(ctx, filter, ref); err != nil {
			return domain.WorkItemTrail{}, err
		}
	}
	return trailFrom(ref, runs, shares, cost, page), nil
}

// workItemSource is the FROM of every trail statement: the card's claims
// joined to the runs the filter reads. Who and where are the run's, from
// the projection the filter was applied to — never the claim row's, which
// gives only the join, the card and the moment of the claim. args holds
// the filter's arguments and the ref, the last of them.
func workItemSource(filter domain.RunFilter, ref domain.WorkItemRef) (string, []any) {
	where, args := runFilterSQL(filter)
	where = whereAnd(where, realRuns)
	args = append(args, string(ref))
	return `
		from run_work_items c
		join (select run_id, agent_id, company_id, area_id, phase, started_at, ended_at, cost_micros
		      from runs ` + where + `) r
		  on r.run_id = c.run_id
		where c.ref = $` + fmt.Sprint(len(args)), args
}

// workItemRunsSQL is one page: the card's runs the filter reads, past the
// cursor when there is one, in the trail's order, limited when a limit is
// given.
func workItemRunsSQL(
	filter domain.RunFilter, ref domain.WorkItemRef, after *domain.WorkItemCursor, limit int,
) (string, []any) {
	source, args := workItemSource(filter, ref)
	sql := `select c.run_id, r.agent_id, r.company_id, r.area_id, c.claimed_at,
	               r.phase, r.started_at, r.ended_at` + source
	if after != nil {
		args = append(args, after.ClaimedAt.UTC(), string(after.RunID))
		sql += fmt.Sprintf(` and (c.claimed_at, c.run_id) > ($%d, $%d)`, len(args)-1, len(args))
	}
	sql += ` order by c.claimed_at, c.run_id`
	if limit > 0 {
		args = append(args, limit)
		sql += fmt.Sprintf(` limit $%d`, len(args))
	}
	return sql, args
}

func (p *Postgres) workItemRuns(
	ctx context.Context, filter domain.RunFilter, ref domain.WorkItemRef, after *domain.WorkItemCursor, limit int,
) ([]domain.WorkItemRun, error) {
	sql, args := workItemRunsSQL(filter, ref, after, limit)
	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("read the work item's runs: %w", err)
	}
	defer rows.Close()

	out := []domain.WorkItemRun{}
	for rows.Next() {
		var r domain.WorkItemRun
		var ended *time.Time
		if err := rows.Scan(&r.RunID, &r.AgentID, &r.Scope.Company, &r.Scope.Area,
			&r.ClaimedAt, &r.Phase, &r.StartedAt, &ended); err != nil {
			return nil, fmt.Errorf("scan a work item's run: %w", err)
		}
		if ended != nil {
			r.EndedAt = *ended
		}
		r.Link = domain.WorkItemLinkClaim
		out = append(out, r)
	}
	return out, rows.Err()
}

// workItemShares is the role summary: the card's earliest and latest
// opening moments, then the aggregate between them. A card nobody the
// caller can read claimed has no moments, and no summary to read.
func (p *Postgres) workItemShares(
	ctx context.Context, filter domain.RunFilter, ref domain.WorkItemRef,
) ([]domain.WorkItemRoleShare, error) {
	source, args := workItemSource(filter, ref)
	var lo, hi *time.Time
	if err := p.pool.QueryRow(ctx, `select min(r.started_at), max(r.started_at)`+source, args...).
		Scan(&lo, &hi); err != nil {
		return nil, fmt.Errorf("read the work item's span: %w", err)
	}
	if lo == nil || hi == nil {
		return []domain.WorkItemRoleShare{}, nil
	}
	sql, args := workItemRolesSQL(filter, ref, *lo, *hi)
	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("read the work item's roles: %w", err)
	}
	defer rows.Close()
	out := []domain.WorkItemRoleShare{}
	for rows.Next() {
		var s domain.WorkItemRoleShare
		var ended *time.Time
		if err := rows.Scan(&s.AgentID, &s.Scope.Company, &s.Scope.Area, &s.Runs, &s.FirstClaimedAt, &ended,
			&s.CostMicros, &s.Approvals.Requested, &s.Approvals.Approved, &s.Approvals.Refused,
			&s.Approvals.Abandoned); err != nil {
			return nil, fmt.Errorf("scan a work item's role: %w", err)
		}
		if ended != nil {
			s.LastEndedAt = *ended
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// workItemRolesSQL aggregates the card's runs per agent and scope, with the
// approval steps counted in the database. Each step is matched to its run by
// (run_id, opened_at), and opened_at — the partition key — is bounded by the
// card's own first and last opening moments, so only their months are read.
// The decision's verdict is read here; no payload leaves the database.
func workItemRolesSQL(filter domain.RunFilter, ref domain.WorkItemRef, lo, hi time.Time) (string, []any) {
	source, args := workItemSource(filter, ref)
	args = append(args, lo.UTC(), hi.UTC())
	return fmt.Sprintf(`
		with trail as (
			select r.run_id, r.agent_id, r.company_id, r.area_id, r.started_at, r.ended_at,
			       r.cost_micros, c.claimed_at
			%s
		),
		asked as (
			select s.run_id,
			       count(*) filter (where s.kind = 'approval_requested') as requested,
			       count(*) filter (where s.kind = 'approval_decided'
			                          and coalesce((s.payload->>'approved')::boolean, false)) as approved,
			       count(*) filter (where s.kind = 'approval_decided'
			                          and not coalesce((s.payload->>'approved')::boolean, false)) as refused,
			       count(*) filter (where s.kind = 'abandoned') as abandoned
			from run_steps s
			join trail t on t.run_id = s.run_id and t.started_at = s.opened_at
			where s.opened_at >= $%d and s.opened_at <= $%d
			  and s.kind in ('approval_requested', 'approval_decided', 'abandoned')
			group by s.run_id
		)
		select t.agent_id, t.company_id, t.area_id, count(*)::int, min(t.claimed_at), max(t.ended_at),
		       coalesce(sum(t.cost_micros), 0)::bigint,
		       coalesce(sum(a.requested), 0)::int, coalesce(sum(a.approved), 0)::int,
		       coalesce(sum(a.refused), 0)::int, coalesce(sum(a.abandoned), 0)::int
		from trail t left join asked a on a.run_id = t.run_id
		group by t.agent_id, t.company_id, t.area_id`, source, len(args)-1, len(args)), args
}
