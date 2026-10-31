package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
recordWorkItemClaim judges and projects a claim inside the append's
transaction. Any other step passes through untouched.

Lock first, judge second. The run's own lock is already held by the append,
which settles "one card per run". "One open run per card per agent" spans runs,
so it takes a lock of its own on (agent, card) — an advisory lock, because the
card may have no row yet for a row lock to land on — and only then reads who
holds it, in a separate statement. Under read committed that statement takes a
fresh snapshot after the wait: a rival that committed while this one queued is
seen. Read in the same statement as the lock, or before it, the judgement would
answer for a moment the rival had not committed yet, and both would win.

Lock order is always the run first, then the card, and nothing takes them the
other way round, so two claimers cannot deadlock.
*/
func recordWorkItemClaim(ctx context.Context, tx pgx.Tx, s domain.Step) error {
	if s.Kind != domain.StepWorkItemClaimed {
		return nil
	}
	ref, err := claimedRef(s)
	if err != nil {
		return err
	}
	simulated, err := runSimulated(ctx, tx, s.RunID)
	if err != nil {
		return err
	}
	if simulated {
		return judgeSimulatedClaim(ctx, tx, s, ref)
	}
	if _, err := tx.Exec(ctx,
		`select pg_advisory_xact_lock(hashtextextended('work_item:' || $1::text || chr(31) || $2::text, 0))`,
		string(s.AgentID), string(ref),
	); err != nil {
		return fmt.Errorf("lock work item: %w", err)
	}
	own, heldElsewhere, err := claimHolders(ctx, tx, s, ref)
	if err != nil {
		return err
	}
	if err := judgeClaim(ref, own, heldElsewhere); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		insert into run_work_items (
			run_id, company_id, area_id, agent_id, ref, claimed_seq, claimed_at
		) values ($1, $2, $3, $4, $5, $6, $7)
		on conflict (run_id) do nothing`,
		string(s.RunID), string(s.Scope.Company), string(s.Scope.Area),
		string(s.AgentID), string(ref), s.Seq, s.At,
	); err != nil {
		return fmt.Errorf("project work item claim: %w", err)
	}
	return nil
}

// claimHolders reads, once the lock is held, the card this run already holds
// and whether another open run of the agent holds the one being claimed. The
// phase is the runs projection's, so an ended run is simply not a holder.
func claimHolders(
	ctx context.Context, tx pgx.Tx, s domain.Step, ref domain.WorkItemRef,
) (domain.WorkItemRef, bool, error) {
	var own string
	var heldElsewhere bool
	err := tx.QueryRow(ctx, `
		select
			coalesce((select ref from run_work_items where run_id = $1), ''),
			exists (
				select 1 from run_work_items c join runs r on r.run_id = c.run_id
				where c.agent_id = $2 and c.ref = $3 and c.run_id <> $1
				  and r.phase not in ('finished', 'failed'))`,
		string(s.RunID), string(s.AgentID), string(ref),
	).Scan(&own, &heldElsewhere)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("judge work item claim: %w", err)
	}
	return domain.WorkItemRef(own), heldElsewhere, nil
}

// WorkItemCost is what every run that claimed the card cost, whatever became
// of it. The runs projection carries each run's whole spend, planning
// included, so a run that failed before its first write still counts.
func (p *Postgres) WorkItemCost(
	ctx context.Context, ref domain.WorkItemRef,
) (domain.WorkItemCost, error) {
	out := domain.WorkItemCost{Ref: ref}
	if err := p.pool.QueryRow(ctx, `
		select count(*), coalesce(sum(r.cost_micros), 0)::bigint
		from run_work_items c join runs r on r.run_id = c.run_id
		where c.ref = $1`, string(ref),
	).Scan(&out.Runs, &out.Micros); err != nil {
		return domain.WorkItemCost{}, fmt.Errorf("work item cost: %w", err)
	}
	return out, nil
}

/*
A simulation rehearses the claim and never takes one.

A simulated run is a real run in this ledger with its tool layer replaced, and
the claim never reaches a tool layer — the engine executes it — so the dry
boundary is drawn here, on the run's own durable mark (runs.simulated, set from
its run_started step), where no wiring can forget it. Left durable, a simulated
run parked for approval would hold the card against the real run of the same
agent for as long as nobody answered, and its spend would land on the card.

Inside its own run the claim still behaves, judged from its own trail: the
first claim succeeds, a retry of it succeeds, a second card is refused. Nothing
is projected and no other run is consulted or blocked.
*/
func runSimulated(ctx context.Context, tx pgx.Tx, runID domain.RunID) (bool, error) {
	var simulated bool
	if err := tx.QueryRow(ctx,
		`select simulated from runs where run_id = $1`, string(runID),
	).Scan(&simulated); err != nil {
		return false, fmt.Errorf("read whether the run is simulated: %w", err)
	}
	return simulated, nil
}

func judgeSimulatedClaim(ctx context.Context, tx pgx.Tx, s domain.Step, ref domain.WorkItemRef) error {
	var own string
	if err := tx.QueryRow(ctx, `
		select coalesce((
			select payload->>'ref' from run_steps
			where run_id = $1 and kind = 'work_item_claimed' and seq < $2
			order by seq limit 1), '')`,
		string(s.RunID), s.Seq,
	).Scan(&own); err != nil {
		return fmt.Errorf("judge simulated work item claim: %w", err)
	}
	return judgeClaim(ref, domain.WorkItemRef(own), false)
}
