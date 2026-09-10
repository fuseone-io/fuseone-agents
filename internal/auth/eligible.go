package auth

import (
	"context"
	"fmt"

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

// ApproversNamed lists who may decide in a scope, with their display names.
//
// The same predicate as ApproversIn, which the fan-out uses to decide who is
// actually messaged: a screen offering somebody the list must offer the list
// that will be used, or naming a person there produces a message that never
// goes out and no explanation of why.
func (p *Postgres) ApproversNamed(
	ctx context.Context, scope domain.Scope,
) ([]Eligible, error) {
	rows, err := p.pool.Query(ctx, `
		select distinct g.principal_id, coalesce(pr.display, g.principal_id)
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
		string(domain.RoleApprover), string(domain.Installation),
		string(scope.Company), string(scope.Area))
	if err != nil {
		return nil, fmt.Errorf("auth: who may approve in %s: %w", scope, err)
	}
	defer rows.Close()

	var out []Eligible
	for rows.Next() {
		var one Eligible
		if err := rows.Scan(&one.ID, &one.Display); err != nil {
			return nil, err
		}
		out = append(out, one)
	}
	return out, rows.Err()
}
