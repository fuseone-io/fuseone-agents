package domain

import (
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
			if got := EventCyclesThrough(tc.edges, through(tc.through)); !reflect.DeepEqual(got, tc.want) {
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

	if got := EventCyclesThrough(edges, "a"); got != nil {
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

	if got := EventCyclesThrough(edges, "a"); len(got) != MaxEventCycles {
		t.Errorf("cycles = %d, want the bound %d", len(got), MaxEventCycles)
	}
}
