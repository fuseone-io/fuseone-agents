package auth

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
)

/*
Who an agent's owner may name, and nothing else about them.

Naming somebody to be told about an approval needs one thing on a screen: the
people who can already answer it in that scope, by name. It does not need the
directory, and it must not need authority over the directory — an author
publishing an agent holds no identity administration, and pointing that screen
at the administrative listing gave them an empty control and a refusal nobody
showed.

So this answers the narrowest possible question: the people holding Approver in
a scope, with the name a person would recognise. No grants, no email, no
accounts, no disabled principals, nobody who is not a person.
*/
type Eligible struct {
	ID      domain.UserID
	Display string
}

/*
ApproversNamed lists who may decide in a scope, with their display names.

The same call the fan-out makes, with the names kept: a screen offering somebody
the list must offer the list that will be used, or naming a person there
produces a message that never goes out and no explanation of why. Written as its
own query it was a copy of the predicate, and a copy of a predicate is a
predicate that drifts — invisibly, because both halves keep working and only
disagree about who.
*/
func (p *Postgres) ApproversNamed(
	ctx context.Context, scope domain.Scope,
) ([]Eligible, error) {
	return p.peopleHoldingNamed(ctx, domain.RoleApprover, scope)
}
