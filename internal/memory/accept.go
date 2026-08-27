package memory

import (
	"fmt"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
AcceptInput is one person agreeing to a proposal, possibly in better words.

An options struct rather than six parameters, and the corrected claim lives in
it rather than in a second method: accepting and accepting-with-an-edit are the
same act with the same lock, the same merge and the same event, and two entry
points would be two chances for one of them to grow a rule the other lacks.
*/
type AcceptInput struct {
	ID     string
	Scope  domain.Scope
	By     domain.UserID
	Reason string
	/*
		Claim replaces what the agent proposed, when somebody rewrote it.

		A pointer because absent and empty are different answers. Absent means
		they agreed with the words as well as the fact; empty means they cleared
		the box, and silently keeping the text they just deleted would record an
		agreement to exactly what they refused.
	*/
	Claim *string
	// Override records this even though it looks like it may contain a
	// credential. Same meaning as on creation, and it marks the assertion.
	Override bool
	Now      time.Time
}

func (in AcceptInput) validate() error {
	// The event carries the reason, and recordEvent trims and accepts an empty
	// one — so without this the trail would hold a memory somebody agreed to
	// beside a blank where the justification should be.
	if strings.TrimSpace(in.Reason) == "" {
		return fmt.Errorf("%w: an accept needs a reason", ErrInvalid)
	}
	if in.Claim == nil {
		return nil
	}
	// Refused here for the sentence, not for the protection: a cleared claim
	// reaches Validate as an empty one and is refused there anyway. What this
	// adds is telling the person what to do about it — "omit it" rather than
	// "claim is required", when what they did was empty a box.
	if strings.TrimSpace(*in.Claim) == "" {
		return fmt.Errorf("%w: a corrected claim cannot be empty; omit it to keep the wording",
			ErrInvalid)
	}
	if len(*in.Claim) > domain.MaxMemoryClaimBytes {
		return fmt.Errorf("%w: a claim must fit %d bytes",
			ErrInvalid, domain.MaxMemoryClaimBytes)
	}
	return nil
}

/*
accepted is the assertion a proposal becomes, in the words that were agreed to.

Only the claim may be rewritten. Subject, signature and kind are the identity —
changing one here would not correct this memory, it would create a different one
and mark this proposal accepted for a fact nobody proposed. Somebody who meant
that should dismiss the proposal and teach the other fact, which is two acts
because it is two decisions.

Routed through Validate because assertionFromSuggestion never did: a proposal
recorded before a rule existed, or one whose claim somebody replaced with
something the store will not take, reached the merge unchecked.
*/
func accepted(s domain.MemorySuggestion, in AcceptInput) (domain.MemoryAssertion, error) {
	a := assertionFromSuggestion(s, s.Observations, in.By, in.Now)
	if in.Claim != nil {
		a.Claim = clean(*in.Claim)
	}
	if err := a.Validate(); err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	// On the finished assertion, not on the request. Accepting without
	// rewriting produces a claim, a subject and a signature that came from a
	// row read under the lock — and the queue is not scanned when a proposal is
	// recorded, so this is the first and only place a key in one of them is
	// seen.
	return SecretDecision(a, in.Override, in.Reason)
}
