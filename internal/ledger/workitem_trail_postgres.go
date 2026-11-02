package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
WorkItemTrail reads a card's trail in at most two statements, none of
which grows with the card's history.

Every statement resolves the reference asked for and reads from the same
source — see workItemSource: the claims of the card and of its readable
aliases (#204, slice 2b), by run_work_items' (ref) index, joined to the runs
projection narrowed by the caller's own filter and stripped of simulations.
The filter is the generic one, applied to runs in a subquery so its columns
stay those of runs alone; the card is matched here, by an explicit predicate,
never by teaching the generic filter about work items.

A page is one statement, limited to the page plus one row, that also answers
what the trail resolved to. The first page adds the role summary: one
aggregate grouped by agent and scope that counts the approval steps in SQL,
bounded on run_steps' partition key by the card's earliest and latest opening
moments as the same statement computes them — one snapshot, so a run is never
counted without its approvals. Only the months the card's runs opened in are
read, and no list of runs ever leaves the database or comes back into it.
*/
func (p *Postgres) WorkItemTrail(
	ctx context.Context, filter domain.RunFilter, cost *domain.RunFilter,
	ref domain.WorkItemRef, page domain.WorkItemPage,
) (domain.WorkItemTrail, error) {
	if page.After != nil {
		// A later page has no summary to agree with: one statement, its own
		// snapshot.
		target, runs, err := workItemRuns(ctx, p.pool, filter, ref, page.After, page.Size()+1)
		if err != nil {
			return domain.WorkItemTrail{}, err
		}
		return trailFrom(target, runs, nil, cost, page), nil
	}
	target, runs, shares, err := p.workItemFirstPage(ctx, filter, ref, page.Size()+1)
	if err != nil {
		return domain.WorkItemTrail{}, err
	}
	return trailFrom(target, runs, shares, cost, page), nil
}

/*
workItemFirstPage reads the first page and the role summary in one
read-only repeatable-read transaction, so both describe the same runs and
resolve through the same links. Read in two snapshots, a run committed
between them was counted by the summary — with its approvals and cost — and
missing from a timeline that said it had ended, with no next page to reach it
by.
*/
func (p *Postgres) workItemFirstPage(
	ctx context.Context, filter domain.RunFilter, ref domain.WorkItemRef, limit int,
) (trailRef, []domain.WorkItemRun, []domain.WorkItemRoleShare, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return trailRef{}, nil, nil, fmt.Errorf("open the trail's snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	target, runs, err := workItemRuns(ctx, tx, filter, ref, nil, limit)
	if err != nil {
		return trailRef{}, nil, nil, err
	}
	shares, err := workItemShares(ctx, tx, filter, ref)
	if err != nil {
		return trailRef{}, nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return trailRef{}, nil, nil, fmt.Errorf("close the trail's snapshot: %w", err)
	}
	return target, runs, shares, nil
}

// trailQuerier is what the trail's reads run on: the pool, or the
// transaction that holds the first page's snapshot.
type trailQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// workItemRunsSQL is one page: what the reference resolved to, and the
// card's runs the filter reads, past the cursor when there is one, in the
// trail's order, limited when a limit is given. Always at least one row: an
// empty page is the resolution with no run.
func workItemRunsSQL(
	filter domain.RunFilter, ref domain.WorkItemRef, after *domain.WorkItemCursor, limit int,
) (string, []any) {
	with, from, args := workItemSource(filter, ref)
	page := `select c.run_id, r.agent_id, r.company_id, r.area_id, c.claimed_at,
	               r.phase, r.started_at, r.ended_at, c.ref, ` + trailLinkSQL + ` as link,
	               case when c.via <> 'claim' then w.ref end as claimed_ref,
	               case when c.via = 'claim' then o.ref end as inherited_ref` + from
	if after != nil {
		args = append(args, after.ClaimedAt.UTC(), string(after.RunID))
		page += fmt.Sprintf(` and (c.claimed_at, c.run_id) > ($%d, $%d)`, len(args)-1, len(args))
	}
	page += ` order by c.claimed_at, c.run_id`
	if limit > 0 {
		args = append(args, limit)
		page += fmt.Sprintf(` limit $%d`, len(args))
	}
	return `with ` + with + `, page as (` + page + `)
		select k.card, k.resolved_from, p.run_id, coalesce(p.agent_id, ''), coalesce(p.company_id, ''),
		       coalesce(p.area_id, ''), coalesce(p.claimed_at, 'epoch'), coalesce(p.phase, ''),
		       coalesce(p.started_at, 'epoch'), p.ended_at, coalesce(p.ref, ''), coalesce(p.link, ''),
		       coalesce(p.claimed_ref, ''), coalesce(p.inherited_ref, '')
		from card_of k left join page p on true
		order by p.claimed_at, p.run_id`, args
}

func workItemRuns(
	ctx context.Context, q trailQuerier, filter domain.RunFilter, ref domain.WorkItemRef, after *domain.WorkItemCursor, limit int,
) (trailRef, []domain.WorkItemRun, error) {
	sql, args := workItemRunsSQL(filter, ref, after, limit)
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return trailRef{}, nil, fmt.Errorf("read the work item's runs: %w", err)
	}
	defer rows.Close()

	target := trailRef{target: ref}
	out := []domain.WorkItemRun{}
	for rows.Next() {
		var r domain.WorkItemRun
		var runID *domain.RunID
		var ended *time.Time
		if err := rows.Scan(&target.target, &target.from, &runID, &r.AgentID, &r.Scope.Company, &r.Scope.Area,
			&r.ClaimedAt, &r.Phase, &r.StartedAt, &ended, &r.Ref, &r.Link,
			&r.ClaimedRef, &r.InheritedRef); err != nil {
			return trailRef{}, nil, fmt.Errorf("scan a work item's run: %w", err)
		}
		if runID == nil {
			continue
		}
		r.RunID = *runID
		if ended != nil {
			r.EndedAt = *ended
		}
		out = append(out, r)
	}
	return target, out, rows.Err()
}

// workItemShares is the role summary, read in one statement.
func workItemShares(
	ctx context.Context, q trailQuerier, filter domain.RunFilter, ref domain.WorkItemRef,
) ([]domain.WorkItemRoleShare, error) {
	sql, args := workItemRolesSQL(filter, ref)
	rows, err := q.Query(ctx, sql, args...)
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
// approval steps counted in the database, in one statement and so one
// snapshot: the runs, their cost, the span of their opening moments and their
// approvals are all read from the same moment, and a run that commits while
// the summary is read is counted with what it asked or not at all.
//
// Each step is matched to its run by (run_id, opened_at), and opened_at — the
// partition key — is bounded by the span the same statement computed. The
// bound is not known when the statement is planned, so the months outside it
// are pruned while it runs rather than when it is planned. The decision's
// verdict is read here; no payload leaves the database.
func workItemRolesSQL(filter domain.RunFilter, ref domain.WorkItemRef) (string, []any) {
	with, from, args := workItemSource(filter, ref)
	return fmt.Sprintf(`
		with %s,
		trail as (
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
			where s.opened_at >= (select min(started_at) from trail)
			  and s.opened_at <= (select max(started_at) from trail)
			  and s.kind in ('approval_requested', 'approval_decided', 'abandoned')
			group by s.run_id
		)
		select t.agent_id, t.company_id, t.area_id, count(*)::int, min(t.claimed_at), max(t.ended_at),
		       coalesce(sum(t.cost_micros), 0)::bigint,
		       coalesce(sum(a.requested), 0)::int, coalesce(sum(a.approved), 0)::int,
		       coalesce(sum(a.refused), 0)::int, coalesce(sum(a.abandoned), 0)::int
		from trail t left join asked a on a.run_id = t.run_id
		group by t.agent_id, t.company_id, t.area_id`, with, from), args
}
