package domain

import (
	"errors"
	"regexp"
	"strings"
)

/*
The work-item claim: which run is working on which card (NT-013).

Cron picks are not ownership. Each firing gets a fresh idempotency key and the
engine's idempotency is scoped to a run, so two runs of one role can choose the
same card — most easily while the first waits for a person to approve its
write. The claim is the platform's own record of who works on what, held by
the ledger, so the second run is told the card is taken before it spends
anything on it.
*/

// ToolWorkItemClaim records the card a run works on, and refuses it when
// another open run of the same agent already holds that card. This exact id
// is what the note, the role specifications and the tests name.
const ToolWorkItemClaim ToolID = "$fuseone.work_item.claim"

// MaxWorkItemRefBytes bounds a reference. Every canonical form fits well
// inside it; the bound is the backstop the grammar sits behind.
const MaxWorkItemRefBytes = 512

/*
WorkItemRef is a card's identifier, in one of the canonical forms the factory
uses — never a description of the card:

  - github:<owner>/<repo>#<number> — an issue or pull request. Owner and repo
    are limited to the characters GitHub allows and folded to lower case,
    because GitHub treats them case-insensitively and two spellings of one
    repository must be one card. The number is digits, never zero.
  - ghproject:<node id> — an opaque board item id, in the node-id charset.

A grammar rather than a blacklist, because the reference comes from model
output, possibly steered by untrusted content, and lands in an append-only
step and table for good. Free text would carry personal data there for ever,
and invisible or bidi characters would let two references that read the same
on a screen hold the same card twice. Anything a person should read about the
card belongs beside the reference, never in it.
*/
type WorkItemRef string

var (
	githubWorkItem = regexp.MustCompile(
		`^github:([A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?)/([A-Za-z0-9._-]{1,100})#([1-9][0-9]{0,9})$`)
	projectWorkItem = regexp.MustCompile(`^ghproject:[A-Za-z0-9_-]{1,128}$`)
)

// ParseWorkItemRef trims a reference and returns its canonical form, or
// refuses it when it is in none of the forms above.
func ParseWorkItemRef(raw string) (WorkItemRef, bool) {
	ref := strings.TrimSpace(raw)
	if len(ref) > MaxWorkItemRefBytes {
		return "", false
	}
	if projectWorkItem.MatchString(ref) {
		return WorkItemRef(ref), true
	}
	m := githubWorkItem.FindStringSubmatch(ref)
	if m == nil || m[2] == "." || m[2] == ".." {
		return "", false
	}
	owner, repo := strings.ToLower(m[1]), strings.ToLower(m[2])
	return WorkItemRef("github:" + owner + "/" + repo + "#" + m[3]), true
}

// Coordination reports whether a tool is an internal coordination operation:
// it changes only the platform's own record of who works on what, and nothing
// outside it.
//
// The Gate reads this to let such a call through without approval even in a
// tainted run — as an ordinary write it would park for a person before
// reserving, and the race the claim exists to close would reopen. That is
// only safe because the engine executes these ids itself and never hands them
// to a tool server, so the set is explicit and closed: being platform-owned is
// not enough (a ticket answer reaches a person), and no prefix or registration
// can join it.
func (t ToolID) Coordination() bool {
	return t == ToolWorkItemClaim
}

// WorkItemClaimedPayload is the claim as the ledger records it. The agent is
// the step's own; the reference is the only thing the model chose.
type WorkItemClaimedPayload struct {
	Ref WorkItemRef `json:"ref"`
}

var (
	// ErrWorkItemHeld means another open run of the same agent holds the
	// card. The role should pick the next one.
	ErrWorkItemHeld = errors.New("work item is held by another open run of this agent")
	// ErrWorkItemRunHoldsAnother means this run already holds a different
	// card. A run holds at most one, so a run talked into grabbing the queue
	// cannot.
	ErrWorkItemRunHoldsAnother = errors.New("this run already holds a different work item")
	// ErrWorkItemInvalid is a claim step the ledger cannot key: no agent, or
	// a reference that does not parse.
	ErrWorkItemInvalid = errors.New("work item claim is not a valid key")
)

// WorkItemCost is what every run that claimed one card cost, whatever became
// of them — including runs that failed before their first write.
type WorkItemCost struct {
	Ref    WorkItemRef
	Runs   int
	Micros int64
}
