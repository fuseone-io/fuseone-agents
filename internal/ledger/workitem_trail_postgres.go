package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
WorkItemTrail reads a card's trail in at most two statements, whatever its
length.

The runs come from run_work_items by its (ref) index, joined to the runs
projection narrowed by the caller's own filter and stripped of simulations.
The filter is the generic one, applied to runs in a subquery so its columns
stay those of runs alone; the card is matched here, by an explicit predicate,
never by teaching the generic filter about work items.

The first page reads every visible run of the card — the roles cover them all
— and then their approval steps, in one statement addressed by (run id,
opened_at) so it prunes to the months the runs opened in, the way
StartsAndFinishes does. A later page reads one page of runs past the cursor
and no steps: the roles came with the first.
*/
func (p *Postgres) WorkItemTrail(
	ctx context.Context, filter domain.RunFilter, cost *domain.RunFilter,
	ref domain.WorkItemRef, page domain.WorkItemPage,
) (domain.WorkItemTrail, error) {
	limit := 0
	if page.After != nil {
		limit = page.Size() + 1
	}
	facts, err := p.workItemRuns(ctx, filter, ref, page.After, limit)
	if err != nil {
		return domain.WorkItemTrail{}, err
	}
	if page.After == nil {
		if err := p.workItemApprovals(ctx, facts); err != nil {
			return domain.WorkItemTrail{}, err
		}
	}
	return trailFrom(ref, facts, cost, page), nil
}

// workItemRunsSQL is the runs statement: the card's runs the filter reads,
// past the cursor when there is one, in the trail's order, limited when a
// limit is given.
//
// Who and where are the run's, from the runs projection the filter was
// applied to — never the claim row's. run_work_items supplies the join, the
// card and the moment of the claim, nothing else: a claim row that named
// another agent or scope would otherwise move the run to an area, and its
// cost under a cost:read, the caller was never judged against.
func workItemRunsSQL(
	filter domain.RunFilter, ref domain.WorkItemRef, after *domain.WorkItemCursor, limit int,
) (string, []any) {
	where, args := runFilterSQL(filter)
	where = whereAnd(where, realRuns)
	args = append(args, string(ref))
	sql := `
		select c.run_id, r.agent_id, r.company_id, r.area_id, c.claimed_at,
		       r.phase, r.started_at, r.ended_at, r.cost_micros
		from run_work_items c
		join (select run_id, agent_id, company_id, area_id, phase, started_at, ended_at, cost_micros
		      from runs ` + where + `) r
		  on r.run_id = c.run_id
		where c.ref = $` + fmt.Sprint(len(args))
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
) ([]domain.WorkItemRunFacts, error) {
	sql, args := workItemRunsSQL(filter, ref, after, limit)
	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("read the work item's runs: %w", err)
	}
	defer rows.Close()

	out := []domain.WorkItemRunFacts{}
	for rows.Next() {
		var f domain.WorkItemRunFacts
		var ended *time.Time
		if err := rows.Scan(&f.Run.RunID, &f.Run.AgentID, &f.Run.Scope.Company, &f.Run.Scope.Area,
			&f.Run.ClaimedAt, &f.Run.Phase, &f.Run.StartedAt, &ended, &f.CostMicros); err != nil {
			return nil, fmt.Errorf("scan a work item's run: %w", err)
		}
		if ended != nil {
			f.Run.EndedAt = *ended
		}
		f.Run.Link = domain.WorkItemLinkClaim
		out = append(out, f)
	}
	return out, rows.Err()
}

// workItemApprovalsSQL counts, per run, the approval steps of a batch of
// runs. Each run is addressed by its id and the moment it opened — the
// partition key — so the read touches only the months the batch opened in.
// The decision's verdict is read in the database; no payload leaves it.
const workItemApprovalsSQL = `
	select run_id,
	       count(*) filter (where kind = 'approval_requested'),
	       count(*) filter (where kind = 'approval_decided'
	                          and coalesce((payload->>'approved')::boolean, false)),
	       count(*) filter (where kind = 'approval_decided'
	                          and not coalesce((payload->>'approved')::boolean, false)),
	       count(*) filter (where kind = 'abandoned')
	from run_steps
	where run_id = any($1) and opened_at = any($2)
	  and (run_id, opened_at) in (select * from unnest($1::text[], $2::timestamptz[]))
	  and kind in ('approval_requested', 'approval_decided', 'abandoned')
	group by run_id`

func (p *Postgres) workItemApprovals(ctx context.Context, facts []domain.WorkItemRunFacts) error {
	if len(facts) == 0 {
		return nil
	}
	ids := make([]string, len(facts))
	opened := make([]time.Time, len(facts))
	at := make(map[domain.RunID]int, len(facts))
	for i, f := range facts {
		ids[i], opened[i], at[f.Run.RunID] = string(f.Run.RunID), f.Run.StartedAt, i
	}
	rows, err := p.pool.Query(ctx, workItemApprovalsSQL, ids, opened)
	if err != nil {
		return fmt.Errorf("read the work item's approvals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id domain.RunID
		var a domain.WorkItemApprovals
		if err := rows.Scan(&id, &a.Requested, &a.Approved, &a.Refused, &a.Abandoned); err != nil {
			return fmt.Errorf("scan a run's approvals: %w", err)
		}
		if i, ok := at[id]; ok {
			facts[i].Approvals = a
		}
	}
	return rows.Err()
}
