package ledger

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
A run's inherited work item, as both stores read it (#204, slice 3).

It is sealed on run_started and judged while that step is sealed, under the
same serialisation as the append. Attribution only, which is why nothing here
touches the claim: no lock on the card, no row in run_work_items, no question
asked of who holds it. A run that inherited a card is judged by the claim
exactly as a run that inherited nothing.
*/

// inheritedWorkItem reads the inheritance a step carries, when it is a
// run_started that carries one, and whether the run is a simulation. It
// refuses one that does not validate, and one sealed anywhere but on the
// run's opening step: an inheritance is what opened the run, and a step
// written while the run is going cannot reattribute it.
//
// A start whose payload does not decode carries no inheritance: the runs
// projection reads it the same way, and refusing it here would refuse starts
// this release did not write.
func inheritedWorkItem(s domain.Step, opening bool) (*domain.RunWorkItem, bool, error) {
	if s.Kind != domain.StepRunStarted {
		return nil, false, nil
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(s.Payload, &started); err != nil || started.WorkItem == nil {
		return nil, false, nil
	}
	item := started.WorkItem
	if err := item.Validate(); err != nil {
		return nil, false, err
	}
	if !opening {
		return nil, false, fmt.Errorf("%w: sealed on a step after the run opened", domain.ErrRunWorkItemInvalid)
	}
	if s.AgentID == "" {
		return nil, false, fmt.Errorf("%w: the run names no agent", domain.ErrRunWorkItemInvalid)
	}
	return item, started.Simulated, nil
}

/*
recordWorkItemOrigin projects a run's inheritance inside the append's
transaction, so a refused one takes its step with it.

Who and where are the run's own: this is the run's opening step, which the
runs projection is written from in the same transaction, so its agent and
scope are the run's by definition. A simulation projects nothing.
*/
func recordWorkItemOrigin(ctx context.Context, tx pgx.Tx, s domain.Step) error {
	item, simulated, err := inheritedWorkItem(s, s.Seq == domain.FirstSeq)
	if err != nil || item == nil || simulated {
		return err
	}
	var fromRun *string
	if item.FromRun != "" {
		run := string(item.FromRun)
		fromRun = &run
	}
	if _, err := tx.Exec(ctx, `
		insert into run_work_item_origins (
			run_id, company_id, area_id, agent_id, ref, source, from_run, started_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict (run_id) do nothing`,
		string(s.RunID), string(s.Scope.Company), string(s.Scope.Area), string(s.AgentID),
		string(item.Ref), string(item.Source), fromRun, s.At,
	); err != nil {
		return fmt.Errorf("project inherited work item: %w", err)
	}
	return nil
}

/*
placeOnTrail says how a run is on target's trail, filling in the moment it
joined, the link and the references, or reports that it is not on it.

A run is on a trail once. By its claim first — the card it claimed, or an
alias of it — and only otherwise by its inheritance, resolved through the
same links. Inherited and claimed on one card, it is there by its claim. Each
row also names the run's other attribution, so a run that inherited one card
and claimed another says so on both trails.
*/
func placeOnTrail(
	run *domain.WorkItemRun, steps []domain.Step, target domain.WorkItemRef, links readableAliases,
) bool {
	company := run.Scope.Company
	origin := originOfRun(steps)
	var held domain.WorkItemRef
	claimed, hasClaim := claimStepOf(steps)
	if hasClaim {
		held, _ = claimedRef(claimed) // claimStepOf returns a valid claim only
	}
	if link, on := trailLinkOf(held, links[aliasKey{company, held}], target); hasClaim && on {
		run.ClaimedAt, run.Link, run.Ref = claimed.At, link, held
		if origin != nil {
			run.InheritedRef = origin.Ref
		}
		return true
	}
	if origin == nil {
		return false
	}
	if _, on := trailLinkOf(origin.Ref, links[aliasKey{company, origin.Ref}], target); !on {
		return false
	}
	run.ClaimedAt, run.Link, run.Ref = steps[0].At, domain.WorkItemLink(origin.Source), origin.Ref
	run.ClaimedRef = held
	return true
}

// originOfRun is the inheritance a run's opening step carries, as the
// memory store reads it from the steps rather than a projection. Simulations
// inherit nothing durable.
func originOfRun(steps []domain.Step) *domain.RunWorkItem {
	if len(steps) == 0 || simulatedRun(steps) {
		return nil
	}
	item, _, err := inheritedWorkItem(steps[0], true)
	if err != nil {
		return nil
	}
	return item
}
