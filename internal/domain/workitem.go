package domain

import (
	"errors"
	"strings"
	"unicode"
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

// MaxWorkItemRefBytes bounds a reference. A card id or an issue URL fits many
// times over; a model pasting a whole card body into the key does not.
const MaxWorkItemRefBytes = 512

// WorkItemRef is a card reference as the role names it — an issue
// ("github:owner/repo#123") or a board item id. The platform does not
// interpret it beyond identity: two runs hold the same card when their
// references are byte-equal after trimming.
type WorkItemRef string

// ParseWorkItemRef trims a reference and refuses one that cannot be a key:
// empty, oversized, or carrying control characters that would make two
// renderings of one card look different on a screen and equal in the store.
func ParseWorkItemRef(raw string) (WorkItemRef, bool) {
	ref := strings.TrimSpace(raw)
	if ref == "" || len(ref) > MaxWorkItemRefBytes {
		return "", false
	}
	if strings.IndexFunc(ref, unicode.IsControl) >= 0 {
		return "", false
	}
	return WorkItemRef(ref), true
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
