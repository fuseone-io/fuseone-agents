package ledger

import (
	"context"
	"encoding/json"

	"github.com/fuseone/agents/internal/domain"
)

// WorkItemMeasures holds the Postgres contract over the steps themselves:
// the runs the filter reads that opened inside the window, never a
// simulation, one share per run, folded as the Postgres store folds its
// aggregate.
func (m *Memory) WorkItemMeasures(
	ctx context.Context, filter domain.RunFilter, cost *domain.RunFilter, window domain.WorkItemWindow,
) ([]domain.WorkItemWeek, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	links := visibleAliases(m.aliasRows(), aliasReader(filter))
	bounded := measuresFilter(filter, window)
	var shares []domain.WorkItemWeekShare
	for _, steps := range m.runs {
		if len(steps) == 0 || simulatedRun(steps) || !matches(steps[0], bounded) {
			continue
		}
		shares = append(shares, measureShareOf(steps, links))
	}
	return measuresFrom(shares, cost), nil
}

// measureShareOf is one run as a share of its week: its card, resolved
// through the readable links in its own company, and where its cost goes.
func measureShareOf(steps []domain.Step, links readableAliases) domain.WorkItemWeekShare {
	summary := summarise(steps)
	share := domain.WorkItemWeekShare{
		Week: domain.WeekOf(summary.StartedAt), AgentID: summary.AgentID, Scope: summary.Scope, Runs: 1,
		Proposals: proposalsOf(steps),
	}
	if !terminalPhase(summary.Phase) {
		share.OpenRuns = 1
	}
	if summary.PendingApproval != nil {
		share.Proposals.Pending = 1
	}
	link := linkNone
	if claimed, ok := claimStepOf(steps); ok {
		held, _ := claimedRef(claimed) // claimStepOf returns a valid claim only
		if root, aliased := links[aliasKey{summary.Scope.Company, held}]; aliased {
			held = root
		}
		share.Cards, link = []domain.WorkItemRef{held}, linkClaimed
	} else if originOfRun(steps) != nil {
		link = linkInherited
	}
	addCost(&share.Cost, link, summary.Cost.Micros)
	return share
}

// proposalsOf is approvalsOf with what followed each refusal and how many
// approvals a standing grant gave.
func proposalsOf(steps []domain.Step) domain.WorkItemProposals {
	a := approvalsOf(steps)
	p := domain.WorkItemProposals{
		Requested: a.Requested, Approved: a.Approved, Refused: a.Refused, Abandoned: a.Abandoned,
	}
	p.RevisedAfterRefusal, p.RepeatedAfterRefusal, p.Standing = afterRefusalsOf(steps)
	return p
}

// proposal is what a request asked, for comparing with what came after it:
// the tool and the digest of its arguments, never the arguments.
type proposal struct {
	tool   domain.ToolID
	digest string
}

/*
afterRefusalsOf counts, among the run's refusals that name the request they
answer, those after which the run proposed the same tool again with other
arguments (revised) and with the same ones (repeated) — each refusal at most
once in each — and the approvals a standing grant gave. A legacy refusal that
names no request is never counted: what it refused is not recorded.
*/
func afterRefusalsOf(steps []domain.Step) (revised, repeated, standing int) {
	asked := map[int64]proposal{}
	for _, st := range steps {
		if st.Kind == domain.StepApprovalRequested {
			asked[st.Seq] = proposalOf(st)
		}
	}
	for _, st := range steps {
		if st.Kind != domain.StepApprovalDecided {
			continue
		}
		var d domain.ApprovalDecidedPayload
		if json.Unmarshal(st.Payload, &d) != nil {
			continue
		}
		if domain.IsStandingDecision(d) {
			standing++
		}
		refused, named := asked[d.AtSeq]
		if d.Approved || d.AtSeq == 0 || !named {
			continue
		}
		rev, rep := followUps(steps, st.Seq, refused)
		revised, repeated = revised+rev, repeated+rep
	}
	return revised, repeated, standing
}

// followUps reports whether a proposal of the refused tool came after seq,
// with other arguments and with the same ones, as 0 or 1 each.
func followUps(steps []domain.Step, seq int64, refused proposal) (revised, repeated int) {
	for _, st := range steps {
		if st.Kind != domain.StepApprovalRequested || st.Seq <= seq {
			continue
		}
		next := proposalOf(st)
		switch {
		case next.tool != refused.tool:
		case next.digest == refused.digest:
			repeated = 1
		default:
			revised = 1
		}
	}
	return revised, repeated
}

func proposalOf(st domain.Step) proposal {
	var p domain.ApprovalRequestedPayload
	_ = json.Unmarshal(st.Payload, &p) // an unreadable request compares as empty, as SQL reads it
	return proposal{tool: p.Tool, digest: p.ArgsDigest}
}
