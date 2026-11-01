package engine

import "github.com/fuseone/agents/internal/domain"

/*
eventsWithheld is what a finishing run records about the events it will not
publish (#203).

Recorded by the run itself, on its own closing step, by the one writer it
has: the dispatcher that honours the limit reads finished runs and never
writes to them. Both judge from the same sealed start, so the trail and the
dispatch cannot disagree about whether the next agent was meant to start.
*/
func eventsWithheld(start Start, state State) *domain.EventsWithheld {
	if !start.PublishesEvents {
		return nil
	}
	_, withheld := state.Chain.Next(state.RunID)
	return withheld
}
