package ledger

import (
	"github.com/fuseone/agents/internal/domain"
)

/*
The ledger's weekly measures (#201), as both stores assemble them.

Each store reads the runs the caller's filter reaches that opened inside the
window, never a simulation, and aggregates them per week, agent and scope
into shares. What happens next is the same for both, so it is written once:
whether each share's cost is readable — the trail's rule, costAdmits — and the
fold into one row per week and agent, which applies it all or nothing.

Aliases resolve a claimed pull request to its card under the caller's own
scopes only, as the trail reads them: the filter's company, area or agent
narrows the runs counted, never which links may be read.
*/
func measuresFrom(shares []domain.WorkItemWeekShare, cost *domain.RunFilter) []domain.WorkItemWeek {
	for i := range shares {
		shares[i].CostReadable = costAdmits(cost, shares[i].Scope)
	}
	return domain.RoleWeeks(shares)
}

// measuresFilter is the caller's filter bounded to the window, by the run's
// opening.
func measuresFilter(filter domain.RunFilter, window domain.WorkItemWindow) domain.RunFilter {
	filter.Since, filter.Until = window.From, window.To
	return filter
}

// aliasReader is the part of a filter links are read under.
func aliasReader(filter domain.RunFilter) domain.RunFilter {
	return domain.RunFilter{Scopes: filter.Scopes}
}

// runCardLink is how a run is on a card, for where its cost goes.
type runCardLink int

const (
	// linkNone is a run on no card.
	linkNone runCardLink = iota
	// linkInherited is a run on a card only by how it was opened.
	linkInherited
	// linkClaimed is a run that claimed a card or a pull request.
	linkClaimed
)

// addCost puts a run's cost in the part its link says.
func addCost(c *domain.WorkItemWeekCost, link runCardLink, micros int64) {
	switch link {
	case linkClaimed:
		c.OnCardsMicros += micros
	case linkInherited:
		c.InheritedMicros += micros
	default:
		c.OffCardMicros += micros
	}
}
