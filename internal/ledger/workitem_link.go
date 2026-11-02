package ledger

import (
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
The work-item link, as both stores seal it (#204).

A link is a work_item_linked step, and the ledger holds it to the whole fence
while it seals it, under the same serialisation as the append — so a refusal is
an answer and never a step:

  - the payload is the shape every reader decodes (2a);
  - it speaks for its run: the run has started, and the step carries the
    run's own agent and scope;
  - the root is the card the run holds — its first claim — so the run
    supplies the alias and nothing else;
  - the proof is a step of the same run, before the link, and is what it says:
    the run's own successful create_pull_request whose answer named the alias,
    or a person's approval of a link request;
  - and, outside a simulation, against the company's other links: an alias
    has one root for good (the same link again is a success with no new
    effect), and links are one level deep.

The stores differ only in where they read the run and the company's links
from; the judgements are written once, here and in the domain.
*/

// decodeLink reads a link step that would be sealed at seq, refusing one
// whose payload fails the domain's shape or whose proof is not a step written
// before it.
func decodeLink(s domain.Step, seq int64) (domain.WorkItemLinkedPayload, error) {
	var p domain.WorkItemLinkedPayload
	if err := json.Unmarshal(s.Payload, &p); err != nil {
		return p, fmt.Errorf("%w: %w", domain.ErrWorkItemLinkInvalid, err)
	}
	if err := p.Validate(); err != nil {
		return p, err
	}
	if p.Proof.Seq >= seq {
		return p, fmt.Errorf("%w: the proof at seq %d is not before the link at %d",
			domain.ErrWorkItemLinkInvalid, p.Proof.Seq, seq)
	}
	return p, nil
}

// linkSpeaksForRun refuses a link sealed under another agent or scope than
// its run's, as claimSpeaksForRun does for a claim.
func linkSpeaksForRun(s domain.Step, agent domain.AgentID, scope domain.Scope) error {
	if s.AgentID != agent || s.Scope != scope {
		return fmt.Errorf("%w: the link names %s in %s, the run is %s in %s",
			domain.ErrWorkItemLinkInvalid, s.AgentID, s.Scope, agent, scope)
	}
	return nil
}

// linkRootHeld refuses a link whose root is not the card the run holds.
func linkRootHeld(p domain.WorkItemLinkedPayload, held domain.WorkItemRef) error {
	if held == "" {
		return fmt.Errorf("%w: the run holds no card", domain.ErrWorkItemLinkUnclaimed)
	}
	if held != p.Root {
		return fmt.Errorf("%w: the run holds %q, the link names %q", domain.ErrWorkItemLinkUnclaimed, held, p.Root)
	}
	return nil
}

// stepAt is how a store hands the proof judgement a step of the run by seq,
// found or not.
type stepAt func(seq int64) (domain.Step, bool, error)

/*
judgeLinkProof holds the proof to the steps it names. The kind the payload
claims must be the kind of the step at that seq; a returned call must be the
run's own trusted create_pull_request, stamped with the hosted GitHub
provenance and naming the alias; a decision must approve a request, earlier in
the run, for the link itself.
*/
func judgeLinkProof(p domain.WorkItemLinkedPayload, read stepAt) error {
	proof, found, err := read(p.Proof.Seq)
	if err != nil {
		return err
	}
	if !found || proof.Kind != p.Proof.Kind {
		return fmt.Errorf("%w: no %s at seq %d", domain.ErrWorkItemLinkUnproven, p.Proof.Kind, p.Proof.Seq)
	}
	if proof.Kind == domain.StepToolReturned {
		var returned domain.ToolReturnedPayload
		if err := json.Unmarshal(proof.Payload, &returned); err != nil {
			return fmt.Errorf("%w: %w", domain.ErrWorkItemLinkUnproven, err)
		}
		// The labels it was sealed with: an untrusted result proves nothing.
		return domain.JudgePullRequestProof(returned, proof.Labels, p.Alias)
	}
	var decided domain.ApprovalDecidedPayload
	if err := json.Unmarshal(proof.Payload, &decided); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrWorkItemLinkUnproven, err)
	}
	var requested domain.ApprovalRequestedPayload
	if decided.AtSeq >= domain.FirstSeq && decided.AtSeq < proof.Seq {
		request, found, err := read(decided.AtSeq)
		if err != nil {
			return err
		}
		if found && request.Kind == domain.StepApprovalRequested {
			_ = json.Unmarshal(request.Payload, &requested)
		}
	}
	return domain.JudgeApprovalProof(decided, requested, proof.Seq)
}
