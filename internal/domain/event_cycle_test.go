package domain

import (
	"fmt"
	"reflect"
	"testing"
)

func edge(from AgentID, event string, to AgentID) EventEdge {
	return EventEdge{From: from, Event: event, To: to}
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
			if got := EventCyclesThrough(tc.edges, through(tc.through)).Cycles; !reflect.DeepEqual(got, tc.want) {
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

	if got := EventCyclesThrough(edges, "a").Cycles; got != nil {
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
	want := EventCyclesThrough(edges, "a")
	for i := range edges {
		rotated := append(append([]EventEdge{}, edges[i:]...), edges[:i]...)
		if got := EventCyclesThrough(rotated, "a"); !reflect.DeepEqual(got, want) {
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

	got := EventCyclesThrough(edges, "a")
	if len(got.Cycles) != MaxEventCycles || !got.Truncated {
		t.Errorf("cycles = %d (truncated %v), want the bound %d, said to be truncated",
			len(got.Cycles), got.Truncated, MaxEventCycles)
	}
}

// numbered is n agents named so their order is their number.
func numbered(n int) []AgentID {
	out := make([]AgentID, n)
	for i := range out {
		out[i] = AgentID(fmt.Sprintf("a%02d", i))
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
func TestEventCyclesThrough_aDenseAcyclicGraph_expandsNothing(t *testing.T) {
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
			got := EventCyclesThrough(edges, asked)
			if got.Expansions != 0 || len(got.Cycles) != 0 || got.Truncated {
				t.Errorf("%d agents, through %s: %+v; want no cycle and no expansion", n, asked, got)
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
func TestEventCyclesThrough_aDenseCyclicGraph_stopsAtTheExpansionBound(t *testing.T) {
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

	got := EventCyclesThrough(edges, "a")

	if got.Expansions > MaxCycleExpansions || !got.Truncated {
		t.Errorf("expansions = %d (truncated %v), want at most %d and said to be truncated",
			got.Expansions, got.Truncated, MaxCycleExpansions)
	}
	// Stopping short of every cycle is not stopping short of saying there is
	// one: after pruning, an edge out of the agent always leads back, and the
	// shortest way back is named.
	shortest := append(EventCycle{"a", cluster[11]}, chain...)
	if !reflect.DeepEqual(got.Cycles, []EventCycle{shortest}) {
		t.Errorf("cycles = %v, want the shortest one, %v", got.Cycles, shortest)
	}
}
