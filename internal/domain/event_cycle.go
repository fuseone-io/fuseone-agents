package domain

import (
	"cmp"
	"slices"
)

/*
EventCycle is a closed path in the static event graph (#227): the first
agent's event starts the second, and so on, and the last one's event starts
the first again. One agent alone is an agent that listens to its own event.

Named from the agent that was asked about, so a warning shown when that agent
is published reads from where its author stands. The run-time bound is
MaxEventHops; this is what an author is told before any run reaches it.
*/
type EventCycle []AgentID

// MaxEventCycles is how many cycles one answer names. A dense graph has more
// than anybody reads, and enumerating all of them would grow with the graph
// for a warning that has made its point by the first few.
const MaxEventCycles = 16

/*
EventCyclesThrough is every elementary cycle that passes through the agent,
shortest first and, at each length, in the order of the agents' names.

Only cycles through this agent: one among other agents is not something
publishing this agent closed. Edges that go nowhere — an event nobody listens
to, a trigger nothing publishes — close nothing and are ignored. Several
events between the same two agents are one step between them, because the
cycle is named by its agents.

Deterministic whatever order the edges come in, because they come from maps
and the answer is read by a person comparing two publishes.
*/
func EventCyclesThrough(edges []EventEdge, agent AgentID) []EventCycle {
	next := successors(edges)
	if len(next[agent]) == 0 {
		return nil
	}
	search := cycleSearch{next: next, agent: agent, onPath: map[AgentID]bool{agent: true}}
	for length := 1; length <= len(next) && len(search.found) < MaxEventCycles; length++ {
		search.length = length
		search.walk([]AgentID{agent})
	}
	return search.found
}

// successors is each agent's distinct listeners, in name order.
func successors(edges []EventEdge) map[AgentID][]AgentID {
	next := map[AgentID][]AgentID{}
	for _, e := range edges {
		if e.From == "" || e.To == "" || slices.Contains(next[e.From], e.To) {
			continue
		}
		next[e.From] = append(next[e.From], e.To)
		if _, known := next[e.To]; !known {
			next[e.To] = nil
		}
	}
	for agent := range next {
		slices.SortFunc(next[agent], cmp.Compare[AgentID])
	}
	return next
}

// cycleSearch finds the cycles of one length, walking listeners in name
// order so the cycles come out in that order too.
type cycleSearch struct {
	next   map[AgentID][]AgentID
	agent  AgentID
	length int
	onPath map[AgentID]bool
	found  []EventCycle
}

func (s *cycleSearch) walk(path []AgentID) {
	for _, to := range s.next[path[len(path)-1]] {
		if len(s.found) >= MaxEventCycles {
			return
		}
		if to == s.agent && len(path) == s.length {
			s.found = append(s.found, slices.Clone(path))
			continue
		}
		if s.onPath[to] || len(path) >= s.length {
			continue
		}
		s.onPath[to] = true
		s.walk(append(path, to))
		s.onPath[to] = false
	}
}
