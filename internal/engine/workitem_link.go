package engine

import (
	"encoding/json"

	"github.com/fuseone/agents/internal/domain"
)

/*
applyWorkItemLink folds a work_item_linked step (#204).

Read, never acted on. A link records a pull request as another name of the
card the run claimed; nothing the engine decides — what to plan, what the Gate
lets through, what the run still owes — depends on it, and the ledger has
already judged it while sealing it.

It is decoded all the same, because a step this release cannot read is a
corrupt record, and the fold says so rather than carrying it silently. What it
does not do is hold the payload to the writer's fence: the fence is judged once,
when the step is sealed, and a fold that re-judged it would fail a run for a
rule a later release is free to refine.
*/
func (s *State) applyWorkItemLink(step domain.Step) error {
	var p domain.WorkItemLinkedPayload
	return decode(step, &p)
}

/*
workItemFacts is what the run's own record says a link may rest on (#204,
slice 2b), folded so the engine can fill a link in and name its proof without
reading the ledger again:

  - the card the run holds: its first claim, which is the only one the ledger
    sealed as a hold — a second card is refused, a retry is the same card;
  - each pull request a create_pull_request of this run answered naming, at
    the seq of that return;
  - each request for the link a person approved, at the seq of the decision.

Noted beside the fold, never instead of it: none of it changes a decision the
loop takes, only what a link proposal is judged and recorded with. A payload
that does not decode notes nothing; the fold proper is what refuses it.
*/
type workItemFacts struct {
	claim         domain.WorkItemRef
	pullRequests  map[domain.WorkItemRef]int64
	linkApprovals map[int64]int64
}

// noteWorkItem runs before the step's own transition, so a decision is noted
// against the request it answers before the fold clears it.
func (s *State) noteWorkItem(step domain.Step) {
	switch step.Kind {
	case domain.StepWorkItemClaimed:
		var p domain.WorkItemClaimedPayload
		if s.workItem.claim == "" && json.Unmarshal(step.Payload, &p) == nil {
			if ref, ok := domain.ParseWorkItemRef(string(p.Ref)); ok && ref == p.Ref {
				s.workItem.claim = ref
			}
		}
	case domain.StepToolReturned:
		var p domain.ToolReturnedPayload
		if json.Unmarshal(step.Payload, &p) == nil && p.PullRequest != "" &&
			domain.JudgePullRequestProof(p, p.PullRequest) == nil {
			if s.workItem.pullRequests == nil {
				s.workItem.pullRequests = map[domain.WorkItemRef]int64{}
			}
			if _, seen := s.workItem.pullRequests[p.PullRequest]; !seen {
				s.workItem.pullRequests[p.PullRequest] = step.Seq
			}
		}
	case domain.StepApprovalDecided:
		s.noteLinkApproval(step)
	}
}

func (s *State) noteLinkApproval(step domain.Step) {
	var p domain.ApprovalDecidedPayload
	if json.Unmarshal(step.Payload, &p) != nil || !p.Approved || s.requested == nil ||
		s.requested.Tool != domain.ToolWorkItemLink || p.AtSeq != s.requested.AtSeq {
		return
	}
	if s.workItem.linkApprovals == nil {
		s.workItem.linkApprovals = map[int64]int64{}
	}
	s.workItem.linkApprovals[p.AtSeq] = step.Seq
}
