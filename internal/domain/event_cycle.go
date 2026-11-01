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
MaxCycleExpansions is how many edges one search may follow.

Bounding the cycles found is not bounding the work: a graph can hold
exponentially many paths that never come back. Anybody who may publish can
shape the graph, so the work a publish does on it is capped here, and an
answer that stopped at the cap says so.
*/
const MaxCycleExpansions = 10_000

// EventCycleSearch is the cycles found, and what finding them cost.
type EventCycleSearch struct {
	Cycles []EventCycle
	// Truncated is a search that stopped at MaxEventCycles or at
	// MaxCycleExpansions: the cycles named may not be all there are.
	Truncated bool
	// Expansions is how many edges the search followed.
	Expansions int
}

/*
EventCyclesThrough is the elementary cycles that pass through the agent,
shortest first and, at each length, in the order of the agents' names.

Only cycles through this agent: one among other agents is not something
publishing this agent closed. Edges that go nowhere — an event nobody listens
to, a trigger nothing publishes — close nothing and are ignored. Several
events between the same two agents are one step between them, because the
cycle is named by its agents.

Only an agent the published one reaches, and that reaches it back, can be on
a cycle through it. Everything else is pruned first, in time linear in the
graph, so a graph with no way back costs no search at all. What remains is
searched within MaxCycleExpansions.

Deterministic whatever order the edges come in, because they come from maps
and the answer is read by a person comparing two publishes.
*/
func EventCyclesThrough(edges []EventEdge, agent AgentID) EventCycleSearch {
	next := onCyclesThrough(successors(edges), agent)
	if len(next[agent]) == 0 {
		return EventCycleSearch{}
	}
	search := cycleSearch{next: next, agent: agent, onPath: map[AgentID]bool{agent: true}}
	for length := 1; length <= len(next) && !search.stopped(); length++ {
		search.length = length
		search.walk([]AgentID{agent})
	}
	found := search.found
	if len(found) == 0 {
		// Pruned, every edge out of the agent leads back to it, so there is a
		// cycle even when the bound came before the search reached its length.
		found = []EventCycle{shortestCycle(next, agent)}
	}
	return EventCycleSearch{Cycles: found, Truncated: search.stopped(), Expansions: search.expansions}
}

// shortestCycle is the shortest way from the agent back to itself, breadth
// first in name order, so the same graph always names the same one. Asked
// only of a pruned graph with an edge out of the agent, where one exists.
func shortestCycle(next map[AgentID][]AgentID, agent AgentID) EventCycle {
	parent := map[AgentID]AgentID{}
	queue := []AgentID{agent}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		for _, to := range next[at] {
			if to == agent {
				return pathTo(parent, agent, at)
			}
			if _, seen := parent[to]; !seen {
				parent[to] = at
				queue = append(queue, to)
			}
		}
	}
	return nil
}

// pathTo walks the parents back from the last agent to the first.
func pathTo(parent map[AgentID]AgentID, first, last AgentID) EventCycle {
	cycle := EventCycle{last}
	for at := last; at != first; {
		at = parent[at]
		cycle = append(cycle, at)
	}
	slices.Reverse(cycle)
	return cycle
}

// successors is each agent's distinct listeners, in name order.
func successors(edges []EventEdge) map[AgentID][]AgentID {
	next := map[AgentID][]AgentID{}
	for _, e := range edges {
		if e.From == "" || e.To == "" || slices.Contains(next[e.From], e.To) {
			continue
		}
		next[e.From] = append(next[e.From], e.To)
	}
	for agent := range next {
		slices.SortFunc(next[agent], cmp.Compare[AgentID])
	}
	return next
}

// onCyclesThrough keeps the agents that are both reached from the agent and
// reach it back, and the edges between them.
func onCyclesThrough(next map[AgentID][]AgentID, agent AgentID) map[AgentID][]AgentID {
	previous := map[AgentID][]AgentID{}
	for from, tos := range next {
		for _, to := range tos {
			previous[to] = append(previous[to], from)
		}
	}
	reached, reaching := reachable(next, agent), reachable(previous, agent)
	kept := map[AgentID][]AgentID{}
	for from := range reached {
		if !reaching[from] {
			continue
		}
		kept[from] = []AgentID{}
		for _, to := range next[from] {
			if reached[to] && reaching[to] {
				kept[from] = append(kept[from], to)
			}
		}
	}
	return kept
}

// reachable is every agent a walk along the edges reaches from the start,
// the start included.
func reachable(edges map[AgentID][]AgentID, start AgentID) map[AgentID]bool {
	seen := map[AgentID]bool{start: true}
	queue := []AgentID{start}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		for _, to := range edges[at] {
			if !seen[to] {
				seen[to] = true
				queue = append(queue, to)
			}
		}
	}
	return seen
}

// cycleSearch finds the cycles of one length, walking listeners in name
// order so the cycles come out in that order too.
type cycleSearch struct {
	next       map[AgentID][]AgentID
	agent      AgentID
	length     int
	onPath     map[AgentID]bool
	found      []EventCycle
	expansions int
}

// stopped is a search that reached either bound.
func (s *cycleSearch) stopped() bool {
	return len(s.found) >= MaxEventCycles || s.expansions >= MaxCycleExpansions
}

func (s *cycleSearch) walk(path []AgentID) {
	for _, to := range s.next[path[len(path)-1]] {
		if s.stopped() {
			return
		}
		s.expansions++
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
