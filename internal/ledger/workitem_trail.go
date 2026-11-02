package ledger

import (
	"github.com/fuseone/agents/internal/domain"
)

/*
A card's trail, as both stores assemble it (#204, slice 1).

Each store reads, already narrowed by the caller's own filter and stripped
of simulations, at most one page of the card's runs plus one past it, in the
trail's order, and — on the first page only — the role shares: the card's
runs aggregated per agent and scope. What happens next is the same for both,
so it is written once: whether each share's cost is readable, the roles, and
the page with its cursor.

Nothing about a page grows with the card's history but the shares, and
those grow with its agents and areas, not its runs.

The cursor the store was given is a position, applied after the caller's
filter. Nothing in it reaches the filter.
*/
func trailFrom(
	ref trailRef, runs []domain.WorkItemRun, shares []domain.WorkItemRoleShare,
	cost *domain.RunFilter, page domain.WorkItemPage,
) domain.WorkItemTrail {
	out := domain.WorkItemTrail{Ref: ref.target, ResolvedFrom: ref.from, Runs: []domain.WorkItemRun{}}
	if page.After == nil {
		for i := range shares {
			shares[i].CostReadable = costAdmits(cost, shares[i].Scope)
		}
		out.Roles = domain.WorkItemRoles(shares)
	}
	size := page.Size()
	for i, r := range runs {
		if i == size {
			out.Next = domain.WorkItemCursorAt(runs[size-1])
			break
		}
		out.Runs = append(out.Runs, r)
	}
	return out
}

// costAdmits reports whether the caller's cost:read filter reaches a scope.
// No filter is no cost:read anywhere, and admits nothing.
func costAdmits(cost *domain.RunFilter, scope domain.Scope) bool {
	switch {
	case cost == nil:
		return false
	case cost.Scope.Company != "" && cost.Scope.Company != scope.Company:
		return false
	case cost.Scope.Area != "" && cost.Scope.Area != scope.Area:
		return false
	case len(cost.Scopes) > 0 && !withinAny(scope, cost.Scopes):
		return false
	}
	return true
}
