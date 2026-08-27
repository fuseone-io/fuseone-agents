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
	// Claim replaces what the agent proposed, when somebody rewrote it. Empty
	// means they agreed with the words as well as the fact.
	Claim string
	Now   time.Time
}

func (in AcceptInput) validate() error {
	// The event carries the reason, and recordEvent trims and accepts an empty
	// one — so without this the trail would hold a memory somebody agreed to
	// beside a blank where the justification should be.
	if strings.TrimSpace(in.Reason) == "" {
		return fmt.Errorf("%w: an accept needs a reason", ErrInvalid)
	}
	if len(in.Claim) > domain.MaxMemoryClaimBytes {
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
	if claim := clean(in.Claim); claim != "" {
		a.Claim = claim
	}
	if err := a.Validate(); err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return a, nil
}
