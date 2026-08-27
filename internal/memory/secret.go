package memory

import (
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

var (
	// ErrSecret means the memory carries a private key or a complete token in
	// a recognised format. Nothing clears it.
	ErrSecret = errors.New("memory: a private key or a complete token was recognised")
	// ErrSecretSuspected means part of it is long and random enough to be a
	// credential, and nobody has said otherwise. A person with publish
	// permission may override it.
	ErrSecretSuspected = errors.New("memory: text long enough and random enough to be a credential")
)

/*
SecretDecision is the whole secret policy, applied to a finished assertion.

Here rather than at the edge, because the edge does not always know what the
memory says. Accepting a proposal without rewriting it produces an assertion
whose claim, subject and signature come from a row read under the lock — and a
check on the request body would have inspected an empty claim and let a key
straight through. So the rule runs where the assertion is complete, whichever
door it came through.

Certain is refused and nothing clears it: the override exists so somebody can
say a guess was wrong, not so a client can opt out of being asked.

An override marks the assertion. The flag is a request field and disappears with
the request; the label is what puts the decision in the row, in the list and in
the event detail, and an override nobody can see later is a guard that quietly
stopped applying.

`also` carries text that is not part of the assertion but travels with it — the
reason, which goes into the event.
*/
func SecretDecision(
	a domain.MemoryAssertion, override bool, also ...string,
) (domain.MemoryAssertion, error) {
	values := append([]string{a.Kind, a.Subject, a.Signature, a.Claim}, also...)
	switch domain.LooksLikeSecret(values...) {
	case domain.SecretCertain:
		return domain.MemoryAssertion{}, fmt.Errorf("%w", ErrSecret)
	case domain.SecretSuspected:
		if !override {
			return domain.MemoryAssertion{}, fmt.Errorf("%w", ErrSecretSuspected)
		}
		a.Labels = a.Labels.Union(domain.NewLabels(domain.LabelSecret))
	}
	return a, nil
}
