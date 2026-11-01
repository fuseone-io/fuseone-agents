package spec

import (
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

/*
EventGraphWith is the published event graph as it would stand with this
version current (#227).

The version replaces its agent's current one, so every edge that agent's
current version drew — out of it and into it — goes, and the edges the draft
declares are joined to the rest: to the agents that listen to what it emits,
and from the agents that emit what it listens to. An agent that listens to
its own event is an edge to itself.

Pure, so the cycle check a publish runs is the graph's arithmetic and not a
read in the middle of a transaction. It draws only edges with both ends,
which is what a cycle is made of; the dangling ones a screen shows are
Registry.Edges' to draw.
*/
func EventGraphWith(published []domain.EventEdge, draft Spec) []domain.EventEdge {
	var out []domain.EventEdge
	for _, e := range published {
		if e.From != draft.ID && e.To != draft.ID {
			out = append(out, e)
		}
	}

	others := rolesBesides(published, draft.ID)
	listens := listenedEvents(draft)
	for _, emitted := range draft.Emits {
		for _, to := range others.listeners[emitted.Event] {
			out = append(out, domain.EventEdge{From: draft.ID, Event: emitted.Event, To: to})
		}
		if slices.Contains(listens, emitted.Event) {
			out = append(out, domain.EventEdge{From: draft.ID, Event: emitted.Event, To: draft.ID})
		}
	}
	for _, event := range listens {
		for _, from := range others.emitters[event] {
			out = append(out, domain.EventEdge{From: from, Event: event, To: draft.ID})
		}
	}
	return out
}

// eventRoles is, per event, the agents that listen to it and the ones that
// emit it.
type eventRoles struct {
	listeners map[string][]domain.AgentID
	emitters  map[string][]domain.AgentID
}

/*
rolesBesides is who listens and who emits, apart from the agent itself.

Read from every edge, the agent's own included: review listening to what dev
emits is drawn only as an edge out of dev, and it is still true of review
after dev's publish.
*/
func rolesBesides(published []domain.EventEdge, agent domain.AgentID) eventRoles {
	roles := eventRoles{listeners: map[string][]domain.AgentID{}, emitters: map[string][]domain.AgentID{}}
	for _, e := range published {
		roles.listeners[e.Event] = addOther(roles.listeners[e.Event], e.To, agent)
		roles.emitters[e.Event] = addOther(roles.emitters[e.Event], e.From, agent)
	}
	return roles
}

// addOther adds an agent once, unless it is absent or the one excluded.
func addOther(agents []domain.AgentID, one, excluded domain.AgentID) []domain.AgentID {
	if one == "" || one == excluded || slices.Contains(agents, one) {
		return agents
	}
	return append(agents, one)
}

// listenedEvents is the events whose publication starts this version.
func listenedEvents(s Spec) []string {
	var out []string
	for _, t := range s.Triggers {
		if t.Type == TriggerEvent && t.Event != "" {
			out = append(out, t.Event)
		}
	}
	return out
}
