package spec_test

import (
	"reflect"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// draft is a version that publishes some events and listens to others.
func draft(id domain.AgentID, emits []string, listens ...string) spec.Spec {
	s := spec.Spec{ID: id}
	for _, e := range emits {
		s.Emits = append(s.Emits, domain.AgentEvent{Event: e})
	}
	for _, e := range listens {
		s.Triggers = append(s.Triggers, spec.Trigger{Type: spec.TriggerEvent, Event: e})
	}
	return s
}

/*
The graph a version would leave behind replaces what its agent's current
version said, both ways.

What the current version emitted and listened to is gone — its edges are the
ones a publish takes away — and what the draft declares is joined to the rest
of the published graph: to the agents listening to what it emits, and from
the agents emitting what it listens to.
*/
func TestEventGraphWith_theDraftReplacesItsAgentsCurrentEdges(t *testing.T) {
	t.Parallel()
	published := []domain.EventEdge{
		{From: "dev", Event: "code.ready", To: "review"},
		{From: "review", Event: "review.done"},
		{From: "dev", Event: "old.event", To: "archive"},
		{From: "intake", Event: "ticket.new", To: "dev"},
		{Event: "nobody.emits", To: "review"},
	}

	got := spec.EventGraphWith(published, draft("dev", []string{"code.ready"}, "review.done"))

	want := domain.EventCyclesThrough(got, "dev")
	if !reflect.DeepEqual(want, []domain.EventCycle{{"dev", "review"}}) {
		t.Fatalf("cycles through the draft = %v, want dev → review → dev", want)
	}
	for _, e := range got {
		if e.Event == "old.event" || e.Event == "ticket.new" {
			t.Errorf("edge %+v survived, though the draft no longer declares it", e)
		}
	}
}

// A draft that listens to its own event closes a cycle of one.
func TestEventGraphWith_aDraftListeningToItself_isAnEdgeToItself(t *testing.T) {
	t.Parallel()

	got := spec.EventGraphWith(nil, draft("dev", []string{"x"}, "x"))

	if !reflect.DeepEqual(got, []domain.EventEdge{{From: "dev", Event: "x", To: "dev"}}) {
		t.Errorf("edges = %+v, want dev → dev", got)
	}
}

// Republishing what is already current changes nothing about the graph.
func TestEventGraphWith_theCurrentVersionAgain_leavesTheCyclesAsTheyWere(t *testing.T) {
	t.Parallel()
	published := []domain.EventEdge{
		{From: "dev", Event: "code.ready", To: "review"},
		{From: "review", Event: "review.done", To: "dev"},
	}

	got := spec.EventGraphWith(published, draft("dev", []string{"code.ready"}, "review.done"))

	if cycles := domain.EventCyclesThrough(got, "dev"); len(cycles) != 1 {
		t.Errorf("cycles = %v, want the one dev already closed", cycles)
	}
}
