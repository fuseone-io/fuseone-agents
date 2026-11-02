package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

/*
The link's writer (#204, slice 2b): $fuseone.work_item.link, and what a link
must show before the ledger seals it.

A Developer run claims an issue and opens a pull request for it. Calling the
link with the pull request's reference records the PR as another name of the
issue, so the runs that later claim the PR — the reviewer, the tester — are on
the issue's trail. The run supplies the alias and nothing else: the root is
the run's own claim, and the proof is a step of the same run.

The proof that needs nobody is the run's own create_pull_request: the call
returned, did not fail, and its answer named this pull request. The engine
reads that answer as it records the return and writes the pull request it
named onto the tool_returned step (ToolReturnedPayload.PullRequest), because
the answer itself lives in the content store, outside the ledger, and the
ledger can only judge what it holds. A link without that proof is an ordinary
write, and its proof is the person who approved it.
*/

// ToolWorkItemLink records a pull request as an alias of the card the run
// holds. This exact id is what the role specifications and the tests name.
const ToolWorkItemLink ToolID = "$fuseone.work_item.link"

// createPullRequest is the tool, on any GitHub server, whose successful answer
// proves a pull request is the run's own.
const createPullRequest = "create_pull_request"

var (
	// ErrWorkItemLinkedElsewhere means the alias is already another card's.
	// A pull request has one root for good; a link never moves it.
	ErrWorkItemLinkedElsewhere = errors.New("the pull request is already linked to another work item")
	// ErrWorkItemLinkUnclaimed means the link's root is not the card this run
	// holds — the run holds none, or another one.
	ErrWorkItemLinkUnclaimed = errors.New("the link's root is not the work item this run holds")
	// ErrWorkItemLinkNested means one side is already the other kind: the
	// root is itself an alias, or the alias already has aliases of its own.
	// Resolution is one level deep, so neither can be linked.
	ErrWorkItemLinkNested = errors.New("work item links are one level deep")
	// ErrWorkItemLinkUnproven means the proof step is not what the link says
	// it is: not this run's successful create_pull_request naming the alias,
	// nor a person's approval of the link.
	ErrWorkItemLinkUnproven = errors.New("the link's proof does not verify")
)

// CreatesPullRequest reports whether the tool is create_pull_request on some
// registered server. The server's name is the installation's choice — one per
// factory role — so the tool is judged by its own name, and a platform tool
// never is one.
func (t ToolID) CreatesPullRequest() bool {
	server, name, found := strings.Cut(string(t), ".")
	return found && server != "" && name == createPullRequest && !t.PlatformOwned()
}

var pullRequestURL = regexp.MustCompile(`^https://github\.com/([^/?#]+)/([^/?#]+)/pull/([0-9]+)$`)

/*
PullRequestNamed is the pull request a create_pull_request answer names: the
web address of the pull request, in the answer's url or html_url — the
minimal answer the hosted GitHub server gives has the first, the full pull
request the second. When both are there they must agree.

Anything else names none: an answer that is not one JSON object, an address
that is not a pull request on github.com, a prefix the content limit cut. None
is never an error; it only means the link has no proof that needs nobody.
*/
func PullRequestNamed(body []byte) (WorkItemRef, bool) {
	var answer map[string]json.RawMessage
	if err := json.Unmarshal(body, &answer); err != nil {
		return "", false
	}
	var named WorkItemRef
	for _, field := range []string{"url", "html_url"} {
		raw, present := answer[field]
		if !present {
			continue
		}
		ref, ok := pullRequestRef(raw)
		if !ok || (named != "" && named != ref) {
			return "", false
		}
		named = ref
	}
	return named, named != ""
}

func pullRequestRef(raw json.RawMessage) (WorkItemRef, bool) {
	var address string
	if err := json.Unmarshal(raw, &address); err != nil {
		return "", false
	}
	m := pullRequestURL.FindStringSubmatch(address)
	if m == nil {
		return "", false
	}
	return ParseWorkItemRef("github:" + m[1] + "/" + m[2] + "#" + m[3])
}

/*
JudgeTrustedPullRequestProof holds a returned call to being the trusted proof of
a link to alias: create_pull_request, answered by the server for this run
rather than from a cache, not failed, and carrying the engine's stamp — the
pull request the strict parser read from the answer, under the hosted GitHub
provenance — naming exactly that pull request.

Two trusts, and this is only the second. The answer's content keeps whatever
label the Curator gave it — untrusted, by default and by the factory's own
instruction — and that label goes on tainting the run, so every later write
still waits for a person. It is not consulted here: what is judged is the
stamp, whose trust comes from the provenance the catalogue established and the
parser that read one address out of the answer, never from the content.
*/
func JudgeTrustedPullRequestProof(returned ToolReturnedPayload, alias WorkItemRef) error {
	stamp := returned.PullRequest
	switch {
	case !returned.Tool.CreatesPullRequest():
		return fmt.Errorf("%w: the proof is a return of %q, not of create_pull_request", ErrWorkItemLinkUnproven, returned.Tool)
	case returned.Failed:
		return fmt.Errorf("%w: the create_pull_request it names failed", ErrWorkItemLinkUnproven)
	case returned.Cached:
		return fmt.Errorf("%w: the create_pull_request it names was answered from a cache", ErrWorkItemLinkUnproven)
	case !stamp.TrustedProof():
		return fmt.Errorf("%w: the create_pull_request it names was not answered by the hosted GitHub server as an App",
			ErrWorkItemLinkUnproven)
	case stamp.Ref != alias:
		return fmt.Errorf("%w: the create_pull_request it names answered %q, not %q",
			ErrWorkItemLinkUnproven, stamp.Ref, alias)
	}
	return nil
}

// LinkArgs is a link's arguments in their one canonical form, {"ref":alias}:
// what the engine proposes, and so what a person approves, for every link.
func LinkArgs(alias WorkItemRef) []byte {
	raw, _ := json.Marshal(map[string]WorkItemRef{"ref": alias})
	return raw
}

// LinkArgsDigest is the digest an approval request records for the link of
// alias — the same form as every args_digest the engine writes — so a store
// can bind a decision to the one pull request it approved without reading the
// arguments themselves.
func LinkArgsDigest(alias WorkItemRef) string {
	sum := sha256.Sum256(LinkArgs(alias))
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}

// JudgeApprovalProof holds a decision to being the proof of a link to alias: a
// person approved the request at requestedSeq, that request was for the link,
// and its arguments named exactly alias. So one decision proves one link: the
// same link again, and no other.
func JudgeApprovalProof(decided ApprovalDecidedPayload, requested ApprovalRequestedPayload, decidedSeq int64, alias WorkItemRef) error {
	switch {
	case !decided.Approved:
		return fmt.Errorf("%w: the decision it names refused", ErrWorkItemLinkUnproven)
	case decided.AtSeq < FirstSeq || decided.AtSeq >= decidedSeq:
		return fmt.Errorf("%w: the decision names no earlier request (at seq %d)", ErrWorkItemLinkUnproven, decided.AtSeq)
	case requested.Tool != ToolWorkItemLink:
		return fmt.Errorf("%w: the approval was for %q, not the link", ErrWorkItemLinkUnproven, requested.Tool)
	case requested.ArgsDigest != LinkArgsDigest(alias):
		return fmt.Errorf("%w: the approval was for another pull request than %q", ErrWorkItemLinkUnproven, alias)
	}
	return nil
}

// JudgeAlias is the one-root rule, once whoever asks knows what the company's
// aliases already say: the alias's current root ("" for none), whether the
// alias is already a root of others, and whether the root is itself an alias.
// The same link again is an idempotent success, whatever else holds.
func JudgeAlias(p WorkItemLinkedPayload, aliasRoot WorkItemRef, aliasHasAliases, rootIsAlias bool) error {
	switch {
	case aliasRoot == p.Root:
		return nil
	case aliasRoot != "":
		return fmt.Errorf("%w: %q is linked to %q", ErrWorkItemLinkedElsewhere, p.Alias, aliasRoot)
	case aliasHasAliases:
		return fmt.Errorf("%w: %q already has aliases of its own", ErrWorkItemLinkNested, p.Alias)
	case rootIsAlias:
		return fmt.Errorf("%w: %q is itself an alias", ErrWorkItemLinkNested, p.Root)
	}
	return nil
}
