package ledger

import (
	"context"
	"encoding/json"

	"github.com/fuseone/agents/internal/domain"
)

// WorkItemTrail holds the Postgres contract over the steps themselves: the
// runs whose first claim names the card, narrowed by the same filter every
// other read applies, never a simulation, in the trail's order.
func (m *Memory) WorkItemTrail(
	ctx context.Context, filter domain.RunFilter, cost *domain.RunFilter,
	ref domain.WorkItemRef, page domain.WorkItemPage,
) (domain.WorkItemTrail, error) {
	if err := ctx.Err(); err != nil {
		return domain.WorkItemTrail{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var facts []domain.WorkItemRunFacts
	for _, steps := range m.runs {
		f, ok := trailFactsOf(steps, filter, ref)
		if !ok || !page.After.Precedes(f.Run) {
			continue
		}
		facts = append(facts, f)
	}
	sortTrail(facts)
	if page.After != nil && len(facts) > page.Size()+1 {
		facts = facts[:page.Size()+1]
	}
	return trailFrom(ref, facts, cost, page), nil
}

// trailFactsOf is one run's facts when it is on the card's trail and the
// filter reads it.
func trailFactsOf(steps []domain.Step, filter domain.RunFilter, ref domain.WorkItemRef) (domain.WorkItemRunFacts, bool) {
	claimed, ok := claimStepOf(steps)
	if !ok || simulatedRun(steps) || !matches(steps[0], filter) {
		return domain.WorkItemRunFacts{}, false
	}
	if held, err := claimedRef(claimed); err != nil || held != ref {
		return domain.WorkItemRunFacts{}, false
	}
	summary := summarise(steps)
	return domain.WorkItemRunFacts{
		Run: domain.WorkItemRun{
			RunID: summary.RunID, AgentID: claimed.AgentID, Scope: claimed.Scope,
			Phase: summary.Phase, StartedAt: summary.StartedAt, EndedAt: summary.EndedAt,
			ClaimedAt: claimed.At, Link: domain.WorkItemLinkClaim,
		},
		CostMicros: summary.Cost.Micros,
		Approvals:  approvalsOf(steps),
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
