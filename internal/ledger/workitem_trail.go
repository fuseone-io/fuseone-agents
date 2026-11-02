package ledger

import (
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

/*
A card's trail, as both stores assemble it (#204, slice 1).

Each store reads the facts of the card's runs the caller may read — already
narrowed by the caller's own filter, simulations excluded — and hands them
here in the trail's order. What happens next is the same for both, so it is
written once: whether each run's cost is readable, the roles (first page
only, over every visible run of the card), and the page with its cursor.

The cursor the store was given is a position, applied after the caller's
filter. Nothing in it reaches the filter.
*/

// trailFrom assembles one page from the visible runs of a card, sorted in
// the trail's order. On the first page facts are every visible run of the
// card; on a later one they are the runs past the cursor, at most one more
// than a page so the store knows whether another follows.
func trailFrom(
	ref domain.WorkItemRef, facts []domain.WorkItemRunFacts, cost *domain.RunFilter, page domain.WorkItemPage,
) domain.WorkItemTrail {
	out := domain.WorkItemTrail{Ref: ref, Runs: []domain.WorkItemRun{}}
	for i := range facts {
		facts[i].CostReadable = costAdmits(cost, facts[i].Run.Scope)
	}
	if page.After == nil {
		out.Roles = domain.WorkItemRoles(facts)
	}
	size := page.Size()
	for i, f := range facts {
		if i == size {
			out.Next = domain.WorkItemCursorAt(facts[size-1].Run)
			break
		}
		out.Runs = append(out.Runs, f.Run)
	}
	return out
}

// costAdmits reports whether the caller's cost:read filter reaches a run's
// scope. No filter is no cost:read anywhere, and admits nothing.
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

// sortTrail puts facts in the trail's order.
func sortTrail(facts []domain.WorkItemRunFacts) {
	slices.SortFunc(facts, func(a, b domain.WorkItemRunFacts) int {
		return domain.CompareWorkItemRuns(a.Run, b.Run)
	})
}
