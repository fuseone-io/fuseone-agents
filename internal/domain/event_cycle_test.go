package domain

import (
	"fmt"
	"reflect"
	"testing"
)

func edge(from AgentID, event string, to AgentID) EventEdge {
	return EventEdge{From: from, Event: event, To: to}
}

// wired is the declarations that draw exactly these edges: each edge gets an
// event of its own, which its From emits and its To listens to. An edge with
// no To is an event nobody listens to; one with no From, a trigger nothing
// publishes.
func wired(edges []EventEdge) EventWiring {
	w := EventWiring{}
	for _, e := range edges {
		event := fmt.Sprintf("%s>%s:%s", e.From, e.To, e.Event)
		if e.From != "" {
			from := w[e.From]
			from.Emits = append(from.Emits, event)
			w[e.From] = from
		}
		if e.To != "" {
			to := w[e.To]
			to.Listens = append(to.Listens, event)
			w[e.To] = to
		}
	}
	return w
}

// declarations is how many events a wiring declares, emitted and listened.
func declarations(w EventWiring) int {
	n := 0
	for _, one := range w {
		n += len(one.Emits) + len(one.Listens)
	}
	return n
}

// through is the agent a case asks about: "a" unless it names another.
func through(agent AgentID) AgentID {
	if agent == "" {
		return "a"
	}
	return agent
}

func TestEventCyclesThrough_closedPaths_namedFromTheAgentInAStableOrder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		through AgentID
		edges   []EventEdge
		want    []EventCycle
	}{
		{
			name:    "two agents listening to each other",
			through: "dev",
			edges:   []EventEdge{edge("dev", "code.ready", "review"), edge("review", "review.done", "dev")},
			want:    []EventCycle{{"dev", "review"}},
		},
		{
			name: "a longer cycle",
			edges: []EventEdge{
				edge("a", "x", "b"), edge("b", "y", "c"), edge("c", "z", "a"),
			},
			want: []EventCycle{{"a", "b", "c"}},
		},
		{
			name:  "an agent listening to its own event",
			edges: []EventEdge{edge("a", "x", "a")},
			want:  []EventCycle{{"a"}},
		},
		{
			name:  "its own event and a loop through another, the cycle of one first",
			edges: []EventEdge{edge("a", "x", "b"), edge("b", "y", "a"), edge("a", "x", "a")},
			want:  []EventCycle{{"a"}, {"a", "b"}},
		},
		{
			name: "a chain that never comes back",
			edges: []EventEdge{
				edge("a", "x", "b"), edge("b", "y", "c"), edge("", "w", "a"), edge("c", "z", ""),
			},
			want: nil,
		},
		{
			name: "several cycles, shortest first, then by name",
			edges: []EventEdge{
				edge("a", "x", "d"), edge("d", "back", "a"),
				edge("a", "x", "b"), edge("b", "y", "c"), edge("c", "z", "a"),
				edge("b", "back", "a"),
			},
			want: []EventCycle{{"a", "b"}, {"a", "d"}, {"a", "b", "c"}},
		},
		{
			name: "two events between the same pair are one cycle of agents",
			edges: []EventEdge{
				edge("a", "x", "b"), edge("a", "x2", "b"), edge("b", "y", "a"),
			},
			want: []EventCycle{{"a", "b"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := EventCyclesThrough(wired(tc.edges), through(tc.through)).Cycles; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("cycles = %v, want %v", got, tc.want)
			}
		})
	}
}

// A cycle among other agents is not one this agent closes: a publish that
// does not take part in it says nothing about it.
func TestEventCyclesThrough_aCycleElsewhere_isNotThisAgents(t *testing.T) {
	t.Parallel()
	edges := []EventEdge{edge("a", "x", "b"), edge("b", "y", "c"), edge("c", "z", "b")}

	if got := EventCyclesThrough(wired(edges), "a").Cycles; got != nil {
		t.Errorf("cycles = %v, want none through a", got)
	}
}

// The same graph in any order answers the same, because the edges come from
// a map and the warning is read by a person comparing two publishes.
func TestEventCyclesThrough_edgesInAnyOrder_answerTheSame(t *testing.T) {
	t.Parallel()
	edges := []EventEdge{
		edge("a", "x", "d"), edge("d", "back", "a"),
		edge("a", "x", "b"), edge("b", "y", "c"), edge("c", "z", "a"), edge("b", "back", "a"),
	}
	want := EventCyclesThrough(wired(edges), "a")
	for i := range edges {
		rotated := append(append([]EventEdge{}, edges[i:]...), edges[:i]...)
		if got := EventCyclesThrough(wired(rotated), "a"); !reflect.DeepEqual(got, want) {
			t.Errorf("rotation %d = %v, want %v", i, got, want)
		}
	}
}

// A dense graph has more cycles than anybody reads; the list stops at a
// bound rather than taking the publish with it.
func TestEventCyclesThrough_aDenseGraph_stopsAtTheBound(t *testing.T) {
	t.Parallel()
	agents := []AgentID{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"}
	var edges []EventEdge
	for _, from := range agents {
		for _, to := range agents {
			if from != to {
				edges = append(edges, edge(from, "e", to))
			}
		}
	}

	got := EventCyclesThrough(wired(edges), "a")
	if len(got.Cycles) != MaxEventCycles || !got.Truncated {
		t.Errorf("cycles = %d (truncated %v), want the bound %d, said to be truncated",
			len(got.Cycles), got.Truncated, MaxEventCycles)
	}
}

// numbered is n agents named so their order is their number.
func numbered(n int) []AgentID {
	out := make([]AgentID, n)
	for i := range out {
		out[i] = AgentID(fmt.Sprintf("a%03d", i))
	}
	return out
}

/*
A dense graph with no cycle costs nothing to search.

Every edge points forward, so nothing comes back to the agent — but a search
that walked every simple path to find that out did work exponential in the
number of agents: 24 of them took eight seconds. Only an agent that can reach
the published one and be reached from it can be on a cycle through it, and
here there is none.
*/
func TestEventCyclesThrough_aDenseAcyclicGraph_costsWorkLinearInItsDeclarations(t *testing.T) {
	t.Parallel()
	for _, n := range []int{24, 40} {
		agents := numbered(n)
		var edges []EventEdge
		for i, from := range agents {
			for _, to := range agents[i+1:] {
				edges = append(edges, edge(from, "e", to))
			}
		}
		for _, asked := range []AgentID{agents[0], agents[n/2], agents[n-1]} {
			w := wired(edges)
			got := EventCyclesThrough(w, asked)
			if len(got.Cycles) != 0 || got.Truncated || got.Work > 2*declarations(w) {
				t.Errorf("%d agents, through %s: work %d, %d cycles, truncated %v; want no cycle, "+
					"and work within twice the %d declarations", n, asked, got.Work, len(got.Cycles),
					got.Truncated, declarations(w))
			}
		}
	}
}

/*
A dense graph whose only cycles are long stops at the work bound, and says so.

Agent a starts any of twelve agents that all start each other, and only the
last of them leads on, through a chain of twenty, back to a. Every agent is
on a cycle through a, so nothing can be pruned, and no cycle is shorter than
23: before reaching that length the search would wander through the twelve in
every order there is.
*/
func TestEventCyclesThrough_aDenseCyclicGraph_stopsAtTheWorkBound(t *testing.T) {
	t.Parallel()
	cluster, chain := numbered(12), numbered(32)[12:]
	edges := []EventEdge{edge(cluster[11], "out", chain[0]), edge(chain[len(chain)-1], "e", "a")}
	for i, from := range cluster {
		edges = append(edges, edge("a", "e", from))
		for j, to := range cluster {
			if i != j {
				edges = append(edges, edge(from, "e", to))
			}
		}
	}
	for i := 1; i < len(chain); i++ {
		edges = append(edges, edge(chain[i-1], "e", chain[i]))
	}

	got := EventCyclesThrough(wired(edges), "a")

	if got.Work > MaxCycleWork || !got.Truncated {
		t.Errorf("work = %d (truncated %v), want at most %d and said to be truncated",
			got.Work, got.Truncated, MaxCycleWork)
	}
	// Stopping short of every cycle is not stopping short of saying there is
	// one: after pruning, an edge out of the agent always leads back, and the
	// shortest way back is named.
	shortest := append(EventCycle{"a", cluster[11]}, chain...)
	if !reflect.DeepEqual(got.Cycles, []EventCycle{shortest}) {
		t.Errorf("cycles = %v, want the shortest one, %v", got.Cycles, shortest)
	}
}

// everyoneTicks is n agents that all emit one event and all listen to it:
// the densest graph two declarations each can draw, every agent starting
// every agent.
func everyoneTicks(n int) EventWiring {
	w := EventWiring{}
	for _, agent := range numbered(n) {
		w[agent] = AgentWiring{Emits: []string{"tick"}, Listens: []string{"tick"}}
	}
	return w
}

/*
A dense graph of many agents costs work bounded by the agents the search
visits, not by the square of the agents in it.

A thousand agents that each start every other are a million agent-to-agent
edges. Building them all, or scanning a list for each to remove duplicates,
is the cubic cost that sat in front of the walk's bound and under the graph
lock; read from the declarations instead, the search pays for the few agents
it visits.
*/
func TestEventCyclesThrough_aThousandAgentsAllStartingEachOther_staysWithinTheBound(t *testing.T) {
	t.Parallel()
	for _, n := range []int{500, 1000} {
		w := everyoneTicks(n)

		got := EventCyclesThrough(w, "a000")

		if len(got.Cycles) != MaxEventCycles || !got.Truncated {
			t.Errorf("%d agents: %d cycles (truncated %v), want %d, said to be truncated",
				n, len(got.Cycles), got.Truncated, MaxEventCycles)
		}
		// Each declaration read twice, the agent's listeners scanned once, and one
		// listener list per agent visited: the agent and the fifteen it loops with.
		if limit := 2*declarations(w) + (MaxEventCycles+2)*n; got.Work > limit {
			t.Errorf("%d agents: work = %d, want at most %d", n, got.Work, limit)
		}
	}
}

// Past the bound the search stops and says so, however the graph is drawn:
// a wiring whose declarations alone exceed it is not read to the end.
func TestEventCyclesThrough_declarationsBeyondTheBound_stopAndSaySo(t *testing.T) {
	t.Parallel()
	agents := numbered(1000)
	var edges []EventEdge
	for _, from := range agents[:400] {
		for _, to := range agents[400:] {
			edges = append(edges, edge(from, "e", to))
		}
	}

	got := EventCyclesThrough(wired(edges), agents[0])

	if !got.Truncated || got.Work > MaxCycleWork {
		t.Errorf("work = %d (truncated %v), want at most %d, said to be truncated",
			got.Work, got.Truncated, MaxCycleWork)
	}
}
