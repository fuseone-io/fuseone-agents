package auth

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
Who this installation knows may decide an approval in a scope.

The platform could always answer whether *this* person may decide, because that
is what a request carries. Asking somebody to decide needs the other direction,
and nothing answered it: telling people a run is waiting means naming them
first.

**Knows is the honest verb.** A grant exists because an operator wrote it or
because somebody signed in and their identity provider asserted one, so a
person who has never signed in is not here — and their absence says nothing
about whether they may decide. This addresses a message; the decision itself is
checked at the button, against the run's own scope, exactly as it is for
somebody who found the run in the console.
*/
func (p *Postgres) ApproversIn(ctx context.Context, scope domain.Scope) ([]domain.UserID, error) {
	return p.peopleHolding(ctx, domain.RoleApprover, scope)
}

/*
peopleHolding lists the people a role reaches over one scope.

The predicate is domain.Scope.Contains written again, in SQL, and the two are
held together by a test that drives both from one table. Widening downwards
only: an installation grant reaches everything, a company grant reaches its
areas, and an area grant reaches nothing but itself — that last one is the
direction that fails silently, because an approver for one team quietly
becoming an approver for the company is not visible in anything but this
clause.

Disabled accounts are excluded. The decision path already refuses them, so
naming one here produces a private message whose button answers that their
account is gone. Only people: a service or agent principal may hold the role
and has nobody to tell.

Ordered, because the answer decides who a message goes to. Once the fan-out is
capped, a list that reordered between sweeps would send a run's approval to one
set of people and its retry to another, with nobody able to say who was meant
to be asked.

That ordering has no accuser here, and saying so is better than a test that
looks like one. `distinct` makes Postgres sort to deduplicate, so the rows come
back in order whether or not anything asked — I removed the ordering and could
not make ten rows arrive unsorted. It becomes observable the moment a cap
truncates the list, which is where the test for it belongs.
*/
func (p *Postgres) peopleHolding(
	ctx context.Context, role domain.Role, scope domain.Scope,
) ([]domain.UserID, error) {
	rows, err := p.pool.Query(ctx, `
		select distinct g.principal_id
		from role_grants g
		join principals pr on pr.principal_id = g.principal_id
		where g.role = $1
		  and pr.disabled_at is null
		  and pr.kind = 'user'
		  and (
		        (g.company_id = $2 and g.area_id = '')
		     or (g.company_id = $3 and g.area_id = '')
		     or (g.company_id = $3 and g.area_id = $4)
		  )
		order by g.principal_id`,
		string(role), string(domain.Installation),
		string(scope.Company), string(scope.Area))
	if err != nil {
		return nil, fmt.Errorf("auth: people holding %s in %s: %w", role, scope, err)
	}
	defer rows.Close()

	var who []domain.UserID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		who = append(who, domain.UserID(id))
	}
	return who, rows.Err()
}
