package ledger

import (
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
The work-item link, as both stores seal it (#204, slice 2a).

A link is a work_item_linked step. This release refuses one whose payload is
not the shape every reader decodes, and nothing more: it projects no alias and
judges no claim, because nothing on it writes a link. The writer, and the
fence it keeps against the record — the root is the run's claim, the proof
step is what it says it is, an alias has one root — come with 2b, which ships
after every worker can read what it seals.
*/

// judgeLink refuses a link step that would be sealed at seq with a payload a
// reader could not trust: one that fails the domain's shape, or whose proof is
// not a step written before it. Any other kind passes.
func judgeLink(s domain.Step, seq int64) error {
	if s.Kind != domain.StepWorkItemLinked {
		return nil
	}
	var p domain.WorkItemLinkedPayload
	if err := json.Unmarshal(s.Payload, &p); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrWorkItemLinkInvalid, err)
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Proof.Seq >= seq {
		return fmt.Errorf("%w: the proof at seq %d is not before the link at %d",
			domain.ErrWorkItemLinkInvalid, p.Proof.Seq, seq)
	}
	return nil
}
