package ledger

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

// WorkItemTrail holds the Postgres contract over the steps themselves: the
// runs whose first claim names the card, narrowed by the same filter every
// other read applies, never a simulation, in the trail's order — a page of
// them plus one, and on the first page a share per run for the roles.
func (m *Memory) WorkItemTrail(
	ctx context.Context, filter domain.RunFilter, cost *domain.RunFilter,
	ref domain.WorkItemRef, page domain.WorkItemPage,
) (domain.WorkItemTrail, error) {
	if err := ctx.Err(); err != nil {
		return domain.WorkItemTrail{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var runs []domain.WorkItemRun
	var shares []domain.WorkItemRoleShare
	for _, steps := range m.runs {
		run, share, ok := trailRunOf(steps, filter, ref)
		if !ok {
			continue
		}
		shares = append(shares, share)
		if page.After.Precedes(run) {
			runs = append(runs, run)
		}
	}
	slices.SortFunc(runs, domain.CompareWorkItemRuns)
	if len(runs) > page.Size()+1 {
		runs = runs[:page.Size()+1]
	}
	return trailFrom(ref, runs, shares, cost, page), nil
}

// trailRunOf is one run and its role share when it is on the card's trail
// and the filter reads it.
func trailRunOf(
	steps []domain.Step, filter domain.RunFilter, ref domain.WorkItemRef,
) (domain.WorkItemRun, domain.WorkItemRoleShare, bool) {
	claimed, ok := claimStepOf(steps)
	if !ok || simulatedRun(steps) || !matches(steps[0], filter) {
		return domain.WorkItemRun{}, domain.WorkItemRoleShare{}, false
	}
	if held, err := claimedRef(claimed); err != nil || held != ref {
		return domain.WorkItemRun{}, domain.WorkItemRoleShare{}, false
	}
	// Who and where are the run's own, as the filter just judged them; the
	// claim gives the card and the moment, nothing else.
	summary := summarise(steps)
	run := domain.WorkItemRun{
		RunID: summary.RunID, AgentID: summary.AgentID, Scope: summary.Scope,
		Phase: summary.Phase, StartedAt: summary.StartedAt, EndedAt: summary.EndedAt,
		ClaimedAt: claimed.At, Link: domain.WorkItemLinkClaim,
	}
	return run, domain.WorkItemRoleShare{
		AgentID: run.AgentID, Scope: run.Scope, Runs: 1, Approvals: approvalsOf(steps),
		FirstClaimedAt: run.ClaimedAt, LastEndedAt: run.EndedAt, CostMicros: summary.Cost.Micros,
	}, true
}

// claimStepOf is the run's first valid claim — the one the Postgres store
// projects, since a later claim is a retry of it or was refused.
func claimStepOf(steps []domain.Step) (domain.Step, bool) {
	for _, st := range steps {
		if st.Kind != domain.StepWorkItemClaimed {
			continue
		}
		if _, err := claimedRef(st); err == nil {
			return st, true
		}
	}
	return domain.Step{}, false
}

// approvalsOf counts what the run asked people and how they answered. The
// decision is read for its verdict alone; nothing else of the payload leaves.
func approvalsOf(steps []domain.Step) domain.WorkItemApprovals {
	var a domain.WorkItemApprovals
	for _, st := range steps {
		switch st.Kind {
		case domain.StepApprovalRequested:
			a.Requested++
		case domain.StepAbandoned:
			a.Abandoned++
		case domain.StepApprovalDecided:
			var d domain.ApprovalDecidedPayload
			if json.Unmarshal(st.Payload, &d) == nil && d.Approved {
				a.Approved++
			} else {
				a.Refused++
			}
		}
	}
	return a
}
