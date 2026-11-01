package domain

import "slices"

/*
EventCycle is a closed path in the static event graph (#227): the first
agent's event starts the second, and so on, and the last one's event starts
the first again. One agent alone is an agent that listens to its own event.

Named from the agent that was asked about, so a warning shown when that agent
is published reads from where its author stands. The run-time bound is
MaxEventHops; this is what an author is told before any run reaches it.
*/
type EventCycle []AgentID

// AgentWiring is what one agent's current version declares about events:
// what its finished runs publish, and what starts it.
type AgentWiring struct {
	Emits   []string
	Listens []string
	// Scope is where the agent lives. Not read by the search, which judges
	// across every scope because a cycle is real wherever its agents are;
	// carried so whoever shows the answer can hide what its reader may not
	// read.
	Scope Scope
}

/*
EventWiring is the static event graph as the agents declare it, one entry per
agent, rather than as edges.

Kept in this form because the edges are a product: every emitter of an event
times every listener of it. A thousand agents that all publish and listen to
one event are two thousand declarations and a million edges, and a judgement
made under the graph lock must not pay for the million.
*/
type EventWiring map[AgentID]AgentWiring

// MaxEventCycles is how many cycles one answer names. A dense graph has more
// than anybody reads, and enumerating all of them would grow with the graph
// for a warning that has made its point by the first few.
const MaxEventCycles = 16

/*
MaxCycleWork is how much one search may do, counted in declarations read,
listeners looked at and edges followed.

Bounding the cycles found is not bounding the work: a graph can hold
exponentially many paths that never come back, and anybody who may publish
can shape the graph. Every step of the search spends from this, the reading
of the declarations included, and an answer that ran out says so. Half is
for the walk; the rest is kept so a search that found nothing in its half can
still name the shortest cycle.
*/
const MaxCycleWork = 100_000

// EventCycleSearch is the cycles found, and what finding them cost.
type EventCycleSearch struct {
	Cycles []EventCycle
	// Truncated is a search that stopped at MaxEventCycles or at
	// MaxCycleWork: the cycles named may not be all there are, and when none
	// are named the graph was not searched to the end.
	Truncated bool
	// Work is how much of MaxCycleWork the search spent.
	Work int
}

/*
EventCyclesThrough is the elementary cycles that pass through the agent,
shortest first and, at each length, in the order of the agents' names.

Only cycles through this agent: one among other agents is not something
publishing this agent closed. An event nobody listens to and a trigger
nothing publishes close nothing. Several events between the same two agents
are one step between them, because the cycle is named by its agents.

Only an agent the published one reaches, and that reaches it back, can be on
a cycle through it. That is found first, in work linear in the declarations;
the walk then reads an agent's listeners only when it reaches the agent.

Deterministic whatever order the declarations come in, because they come from
maps and the answer is read by a person comparing two publishes.
*/
func EventCyclesThrough(w EventWiring, agent AgentID) EventCycleSearch {
	g := newCycleGraph(w, agent, MaxCycleWork/2)
	first, ok := g.successors(agent)
	if !ok {
		return EventCycleSearch{Truncated: true, Work: g.work}
	}
	if len(first) == 0 {
		return EventCycleSearch{Work: g.work}
	}
	search := cycleSearch{graph: g, agent: agent, onPath: map[AgentID]bool{agent: true}}
	for length := 1; length <= len(g.kept) && !search.stopped(); length++ {
		search.length = length
		search.walk([]AgentID{agent})
	}
	found, truncated := search.found, search.stopped()
	if len(found) == 0 {
		// Pruned, every edge out of the agent leads back to it, so there is a
		// cycle even when the bound came before the walk reached its length.
		g.allow(MaxCycleWork)
		if shortest := g.shortestCycle(agent); shortest != nil {
			found = []EventCycle{shortest}
		}
	}
	return EventCycleSearch{Cycles: found, Truncated: truncated || g.exhausted, Work: g.work}
}

// cycleSearch finds the cycles of one length, walking listeners in name
// order so the cycles come out in that order too.
type cycleSearch struct {
	graph  *cycleGraph
	agent  AgentID
	length int
	onPath map[AgentID]bool
	found  []EventCycle
}

// stopped is a search that named enough cycles or ran out of work.
func (s *cycleSearch) stopped() bool {
	return len(s.found) >= MaxEventCycles || s.graph.exhausted
}

func (s *cycleSearch) walk(path []AgentID) {
	next, ok := s.graph.successors(path[len(path)-1])
	if !ok {
		return
	}
	if len(path) == s.length {
		// Long enough: the only way on is back to the agent, so look it up
		// rather than scan every listener for it.
		if _, back := slices.BinarySearch(next, s.agent); back && s.graph.spend() {
			s.found = append(s.found, slices.Clone(path))
		}
		return
	}
	for _, to := range next {
		if s.stopped() || !s.graph.spend() {
			return
		}
		if s.onPath[to] {
			continue
		}
		s.onPath[to] = true
		s.walk(append(path, to))
		s.onPath[to] = false
	}
}
