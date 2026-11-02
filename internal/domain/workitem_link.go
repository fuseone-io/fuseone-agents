package domain

import (
	"errors"
	"fmt"
	"strings"
)

/*
The work-item link: a pull request recorded as an alias of the card a run
claimed (#204).

A Developer run claims an issue, opens a pull request for it, and the PR is the
same piece of work under a second number. The link is what lets the trail of the
issue show the runs that worked on the PR — the reviewer, the tester — without
anybody claiming the issue twice.

The fence the writer keeps (#204, slice 2b), and that this payload has room to
record every part of:

  - the run supplies only the alias; the root is the run's own claim, read
    from the ledger, never from the model;
  - a link is idempotent and immutable — a correction would be a new step,
    and there is none to write: one pull request maps to one root for good,
    and an alias another card already owns is a conflict, not an overwrite;
  - alias and root share owner and repository, so a link can never carry work
    across an installation or a repository;
  - the link names its proof: the step that shows the pull request is this
    run's own — the tool_returned of the create_pull_request that answered with
    its number — or, without one, the approval_decided where a person approved
    the link instead.

The kind ships one release before anything writes it. An older worker that
meets a kind it does not know fails the run, so every reader must already know
this one on the day a writer seals the first.
*/

// WorkItemLinkedPayload is a link as the ledger records it. Every field is
// required, and none of them is free text.
type WorkItemLinkedPayload struct {
	// Alias is the pull request being linked, as github:<owner>/<repo>#<n>
	// in canonical form. The one thing the run supplies.
	Alias WorkItemRef `json:"alias"`
	// Root is the card the alias becomes another name for: the run's claim,
	// copied from its work_item_claimed step by the engine. Recorded rather
	// than looked up, so the step alone says what it linked — a reader of the
	// trail, an export or a replay never has to fold the run to know.
	Root WorkItemRef `json:"root"`
	// Proof is the step of the same run that warrants the link.
	Proof WorkItemLinkProof `json:"proof"`
}

/*
WorkItemLinkProof points at the step that warrants a link, by its place in the
same run and what it was.

Both halves, as StepRef has them, because either alone is weaker than the claim
being made: a sequence says a step was there and not what it said, and a kind
says what happened but not which one. The step itself carries the evidence — the
tool, the result's digest, the person — so nothing is copied here that could
come to disagree with it.

  - tool_returned: the result of a successful create_pull_request this run made,
    whose answer named the alias's number. The link needs no person.
  - approval_decided: a person approved this link. The path a link without the
    first proof takes, because it then keeps human approval.
*/
type WorkItemLinkProof struct {
	Kind StepKind `json:"kind"`
	Seq  int64    `json:"seq"`
}

// ErrWorkItemLinkInvalid is a link step the ledger refuses to record: a
// reference missing or out of form, an alias that is its own root or lives in
// another repository, or a proof that names no step it could be.
var ErrWorkItemLinkInvalid = errors.New("work item link is not valid")

// Validate holds the payload to its shape. Whether the proof step exists in
// the run, and whether the root is the run's claim, are the ledger's to judge
// against the record; this is only what the step must say to be readable.
func (p WorkItemLinkedPayload) Validate() error {
	alias, err := linkSide("alias", p.Alias)
	if err != nil {
		return err
	}
	root, err := linkSide("root", p.Root)
	if err != nil {
		return err
	}
	if p.Alias == p.Root {
		return fmt.Errorf("%w: %q is linked to itself", ErrWorkItemLinkInvalid, p.Alias)
	}
	if alias != root {
		return fmt.Errorf("%w: alias in %s, root in %s; a link stays in one repository",
			ErrWorkItemLinkInvalid, alias, root)
	}
	return p.Proof.validate()
}

func (p WorkItemLinkProof) validate() error {
	if p.Kind != StepToolReturned && p.Kind != StepApprovalDecided {
		return fmt.Errorf("%w: proof kind %q is neither a returned call nor a decision",
			ErrWorkItemLinkInvalid, p.Kind)
	}
	if p.Seq < FirstSeq {
		return fmt.Errorf("%w: proof names no step (seq %d)", ErrWorkItemLinkInvalid, p.Seq)
	}
	return nil
}

// linkSide checks one reference is a GitHub issue or pull request in canonical
// form and returns its owner/repo. A board item has no repository, so it can
// be neither side: the same-repository fence could not be judged.
func linkSide(side string, ref WorkItemRef) (string, error) {
	canonical, ok := ParseWorkItemRef(string(ref))
	if !ok || canonical != ref {
		return "", fmt.Errorf("%w: %s %q is not a canonical reference", ErrWorkItemLinkInvalid, side, ref)
	}
	repo, found := strings.CutPrefix(string(ref), "github:")
	if !found {
		return "", fmt.Errorf("%w: %s %q is not an issue or pull request", ErrWorkItemLinkInvalid, side, ref)
	}
	repo, _, _ = strings.Cut(repo, "#")
	return repo, nil
}
