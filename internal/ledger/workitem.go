package ledger

import (
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
The work-item claim, as both stores read it (#206).

A claim is a work_item_claimed step. The ledger judges it while sealing it,
under the same serialisation as the append itself, so a refusal is an answer
and never a step: one card per run, and one open run per card per agent.

"Open" is the run's phase, read at the moment of the judgement — never a
release written later. A run that finishes or fails stops holding its card
whatever killed it, and a parked run, a run waiting on a person and a run
compensating all keep it: each of them may still act on the card.
*/

// claimedRef reads the reference a claim step carries, refusing one the
// ledger cannot key on. The reference must already be in its canonical form:
// the ledger records what it judged, and judging a trimmed copy of something
// else would make the payload and the projection disagree.
func claimedRef(s domain.Step) (domain.WorkItemRef, error) {
	if s.AgentID == "" {
		return "", fmt.Errorf("%w: the claim names no agent", domain.ErrWorkItemInvalid)
	}
	var p domain.WorkItemClaimedPayload
	if err := json.Unmarshal(s.Payload, &p); err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrWorkItemInvalid, err)
	}
	ref, ok := domain.ParseWorkItemRef(string(p.Ref))
	if !ok || ref != p.Ref {
		return "", fmt.Errorf("%w: reference %q", domain.ErrWorkItemInvalid, p.Ref)
	}
	return ref, nil
}

// judgeClaim is the decision both stores make once they know what this run
// already holds and whether another open run of the agent holds the card.
func judgeClaim(ref, own domain.WorkItemRef, heldElsewhere bool) error {
	switch {
	case own == ref:
		// A retry of the claim this run already holds.
		return nil
	case own != "":
		return fmt.Errorf("%w: it holds %q", domain.ErrWorkItemRunHoldsAnother, own)
	case heldElsewhere:
		return fmt.Errorf("%w: %q", domain.ErrWorkItemHeld, ref)
	}
	return nil
}

// terminalPhase is the phases in which a run holds nothing. The Postgres
// judgement spells the same list in SQL; the contract suite holds them equal.
func terminalPhase(phase string) bool {
	return phase == "finished" || phase == "failed"
}
