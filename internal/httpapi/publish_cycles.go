package httpapi

import (
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
eventCycleWarning is what a publish says about the cycles the version closes
in the static event graph (#227), or nil when it closes none.

A warning and never a refusal: the factory's Developer ⇄ Reviewer loop is a
cycle somebody designed, and the run-time bound (MaxEventHops) is what stops
one that nobody did. This is the authoring-time half — the author learns of
the loop before the first chain reaches the limit, not from the trail of the
run that did.

The cycles are judged by the publisher, not here: inside the transaction that
writes the version, under the lock every change to a current version takes.
Read here, before the write and outside it, two publications that close a
loop only together could each judge the graph from before the other.
*/
func eventCycleWarning(search domain.EventCycleSearch) *openapi.EventCycleWarning {
	// A search that ran out with nothing found is not "no cycle": it says so,
	// with no cycles named and truncated set.
	if len(search.Cycles) == 0 && !search.Truncated {
		return nil
	}
	out := &openapi.EventCycleWarning{
		Cycles: make([][]string, 0, len(search.Cycles)), Truncated: search.Truncated, HopLimit: domain.MaxEventHops,
	}
	for _, cycle := range search.Cycles {
		agents := make([]string, len(cycle))
		for i, agent := range cycle {
			agents[i] = string(agent)
		}
		out.Cycles = append(out.Cycles, agents)
	}
	return out
}
