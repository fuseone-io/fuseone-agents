package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
$fuseone.work_item.link, executed by the engine itself (#204, slice 2b).

The run supplies one thing, the pull request's reference. The root is the card
the run holds, read from its own record and never from the model; no claim
held, no link. The proof is chosen the same way, and the choice is what the
Gate rules on:

  - proven: a create_pull_request of this run returned, not failed and not from
    a cache, and its answer named this pull request. The engine recorded the
    pull request on that tool_returned step when the call came back, so the
    proof is a step the ledger can re-judge. The Gate is told LinkProven and
    lets the link through like the claim, tainted run or not.
  - unproven: anything else. The link is an ordinary write and parks for a
    person; once approved, its proof is that decision.

A link the record already refuses — no card held, a reference that is not a
pull request of the card's repository — is told to the Gate as LinkRefusal and
refused there, so nobody is asked to approve what the ledger would refuse.

The ledger then judges the whole fence again while it seals the step, and what
only it can know — the pull request already belongs to another card, or links
would nest — comes back as an append error and goes to the model as a failed
result, nothing sealed. Like the claim, only the run's single writer may
append, and no tool server is ever asked to answer for it.
*/

const (
	// CodeWorkItemLinkRefInvalid: the arguments name no usable pull request.
	CodeWorkItemLinkRefInvalid = "work_item_link_ref_invalid"
	// CodeWorkItemLinkUnclaimed: the run holds no card to link to.
	CodeWorkItemLinkUnclaimed = "work_item_link_unclaimed"
	// CodeWorkItemLinkedElsewhere: another card already owns the pull request.
	CodeWorkItemLinkedElsewhere = "work_item_linked_elsewhere"
	// CodeWorkItemLinkNested: one side is already the other kind of reference.
	CodeWorkItemLinkNested = "work_item_link_nested"
	// CodeWorkItemLinkUnproven: neither a create_pull_request nor a person
	// warrants the link.
	CodeWorkItemLinkUnproven = "work_item_link_unproven"
)

type linkAnswer struct {
	Linked  bool               `json:"linked"`
	Alias   domain.WorkItemRef `json:"alias,omitempty"`
	Root    domain.WorkItemRef `json:"root,omitempty"`
	Reason  string             `json:"reason,omitempty"`
	Message string             `json:"message"`
}

// proposedLink is the link a proposal would make, the root filled from the
// card the run holds, or why the record refuses it as it stands.
func proposedLink(state State, args []byte) (domain.WorkItemLinkedPayload, linkAnswer, bool) {
	var a claimArgs
	_ = json.Unmarshal(args, &a)
	alias, ok := domain.ParseWorkItemRef(a.Ref)
	if !ok {
		return domain.WorkItemLinkedPayload{}, linkAnswer{Reason: CodeWorkItemLinkRefInvalid,
			Message: "ref must be the pull request as github:owner/repo#123"}, false
	}
	if state.workItem.claim == "" {
		return domain.WorkItemLinkedPayload{}, linkAnswer{Alias: alias, Reason: CodeWorkItemLinkUnclaimed,
			Message: "this run holds no card; claim the card before linking its pull request"}, false
	}
	p := domain.WorkItemLinkedPayload{Alias: alias, Root: state.workItem.claim,
		Proof: domain.WorkItemLinkProof{Kind: domain.StepToolReturned, Seq: domain.FirstSeq}}
	if err := p.Validate(); err != nil {
		return p, linkAnswer{Alias: alias, Root: p.Root, Reason: CodeWorkItemLinkRefInvalid,
			Message: "ref must be a pull request in the same repository as the card this run holds"}, false
	}
	return p, linkAnswer{}, true
}

// canonicalLink puts a link the model proposed into its one canonical form,
// {"ref":alias}, before anything is ruled on or recorded: what the Gate sees,
// what a person approves, and the args_digest a store binds the decision to
// are then the link of that alias and nothing else. A link whose reference
// does not parse is left as it came; the Gate refuses it.
func canonicalLink(p Proposal) Proposal {
	if p.Tool != domain.ToolWorkItemLink {
		return p
	}
	var a claimArgs
	_ = json.Unmarshal(p.Args, &a)
	if alias, ok := domain.ParseWorkItemRef(a.Ref); ok {
		p.Args = domain.LinkArgs(alias)
	}
	return p
}

// linkStanding is what the Gate is told about a proposal: whether a link is
// proven, and why one can never be made. Nothing for any other tool.
func linkStanding(state State, p Proposal) (proven bool, refusal string) {
	if p.Tool != domain.ToolWorkItemLink {
		return false, ""
	}
	link, answer, ok := proposedLink(state, p.Args)
	if !ok {
		return false, answer.Message
	}
	_, proven = state.workItem.pullRequests[link.Alias]
	return proven, ""
}

// linkProven and linkRefusalOf are linkStanding's two halves, as the Gate's
// request takes them.
func linkProven(state State, p Proposal) bool {
	proven, _ := linkStanding(state, p)
	return proven
}

func linkRefusalOf(state State, p Proposal) string {
	_, refusal := linkStanding(state, p)
	return refusal
}

// linkProof names the step that warrants the link: the run's own
// create_pull_request naming it, or the decision that approved this call.
func linkProof(state State, alias domain.WorkItemRef, approvalAtSeq int64) (domain.WorkItemLinkProof, bool) {
	if seq, ok := state.workItem.pullRequests[alias]; ok {
		return domain.WorkItemLinkProof{Kind: domain.StepToolReturned, Seq: seq}, true
	}
	if seq, ok := state.workItem.linkApprovals[approvalAtSeq]; ok && approvalAtSeq != 0 {
		return domain.WorkItemLinkProof{Kind: domain.StepApprovalDecided, Seq: seq}, true
	}
	return domain.WorkItemLinkProof{}, false
}

// linkWorkItem seals the link, or turns the record's refusal into the answer
// the model reads.
func (r *Runner) linkWorkItem(ctx context.Context, state State, start Start, call Call) (State, ToolResult, error) {
	link, refusal, ok := proposedLink(state, call.Args)
	if !ok {
		return r.answerLink(ctx, state, call, refusal)
	}
	proof, proven := linkProof(state, link.Alias, call.ApprovalAtSeq)
	if !proven {
		return r.answerLink(ctx, state, call, linkAnswer{Alias: link.Alias, Root: link.Root,
			Reason:  CodeWorkItemLinkUnproven,
			Message: "this run's create_pull_request did not name this pull request, and no person approved the link"})
	}
	link.Proof = proof
	next, err := r.append(ctx, state, start, domain.Step{
		Kind: domain.StepWorkItemLinked, Labels: state.Labels.Clone(), Payload: mustJSON(link),
	})
	if err == nil {
		return r.answerLink(ctx, next, call, linkAnswer{Linked: true, Alias: link.Alias, Root: link.Root,
			Message: "the pull request is linked to the card this run holds"})
	}
	answer, known := linkRefusal(err)
	if !known {
		return State{}, ToolResult{}, err
	}
	answer.Alias, answer.Root = link.Alias, link.Root
	return r.answerLink(ctx, state, call, answer)
}

// linkRefusal names a ledger refusal for the model. Anything else is not an
// answer about the link and fails the turn.
func linkRefusal(err error) (linkAnswer, bool) {
	switch {
	case errors.Is(err, domain.ErrWorkItemLinkedElsewhere):
		return linkAnswer{Reason: CodeWorkItemLinkedElsewhere,
			Message: "this pull request is already linked to another card; it is not linked again"}, true
	case errors.Is(err, domain.ErrWorkItemLinkNested):
		return linkAnswer{Reason: CodeWorkItemLinkNested,
			Message: "links are one level deep: the card is itself an alias, or the pull request already has aliases"}, true
	case errors.Is(err, domain.ErrWorkItemLinkUnclaimed):
		return linkAnswer{Reason: CodeWorkItemLinkUnclaimed, Message: "this run holds no card to link to"}, true
	case errors.Is(err, domain.ErrWorkItemLinkUnproven):
		return linkAnswer{Reason: CodeWorkItemLinkUnproven, Message: "the link's proof does not verify"}, true
	case errors.Is(err, domain.ErrWorkItemLinkInvalid):
		return linkAnswer{Reason: CodeWorkItemLinkRefInvalid, Message: "the pull request reference is not usable"}, true
	}
	return linkAnswer{}, false
}

func (r *Runner) answerLink(ctx context.Context, state State, call Call, answer linkAnswer) (State, ToolResult, error) {
	body := mustJSON(answer)
	result := ToolResult{
		ResultDigest: ResultDigest(body), ResultBytes: int64(len(body)),
		Failed: !answer.Linked, ErrorCode: answer.Reason,
	}
	if r.deps.Content != nil {
		ref, err := r.deps.Content.Put(ctx, call.RunID, call.Seq, body)
		if err != nil {
			return State{}, ToolResult{}, fmt.Errorf("engine: store link result: %w", err)
		}
		result.ResultRef = ref
	}
	return state, result, nil
}

// pullRequestOf is the pull request a create_pull_request answer named, read
// back from the content store as the call returns, so the tool_returned step
// can carry it as the proof a link is later judged by.
//
// Stamped only when the answer may prove anything: a live, successful,
// trusted result from a server whose registration the catalogue established
// as the hosted GitHub server reached as an App. An untrusted result, a
// server nobody could vouch for, or an answer that cannot be read stamps
// nothing, and a link of it then needs a person.
func (r *Runner) pullRequestOf(ctx context.Context, tool domain.ToolID, result ToolResult) *domain.PullRequestStamp {
	if !tool.CreatesPullRequest() || result.Failed || result.Cached || result.ResultRef == "" || r.deps.Content == nil ||
		result.Labels.HasAny(domain.LabelUntrusted) || result.Provenance != domain.ProvenanceHostedGitHubApp {
		return nil
	}
	body, err := r.deps.Content.Get(ctx, result.ResultRef)
	if err != nil {
		return nil
	}
	ref, ok := domain.PullRequestNamed(body)
	if !ok {
		return nil
	}
	return &domain.PullRequestStamp{Ref: ref, Via: result.Provenance}
}
