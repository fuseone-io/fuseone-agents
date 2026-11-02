package engine

import "github.com/fuseone/agents/internal/domain"

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
