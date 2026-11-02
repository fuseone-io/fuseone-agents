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
The wiring a version would leave behind replaces what its agent's current
version declared, and leaves every other agent as published.

What the current version emitted and listened to is gone, and what the draft
declares is joined to the rest: a cycle with an agent that listens to what
the draft emits, and emits what it listens to, is found.
*/
func TestWiringWith_theDraftReplacesItsAgentsCurrentDeclarations(t *testing.T) {
	t.Parallel()
	published := spec.WiringOf([]spec.Spec{
		draft("dev", []string{"old.event"}, "ticket.new"),
		draft("review", []string{"review.done"}, "code.ready"),
	})

	got := spec.WiringWith(published, draft("dev", []string{"code.ready"}, "review.done"))

	if cycles := domain.EventCyclesThrough(got, "dev").Cycles; !reflect.DeepEqual(cycles, []domain.EventCycle{{"dev", "review"}}) {
		t.Fatalf("cycles through the draft = %v, want dev → review → dev", cycles)
	}
	want := domain.AgentWiring{Emits: []string{"code.ready"}, Listens: []string{"review.done"}}
	if !reflect.DeepEqual(got["dev"], want) {
		t.Errorf("dev = %+v, want only what the draft declares, %+v", got["dev"], want)
	}
	if published["dev"].Emits[0] != "old.event" {
		t.Errorf("the published wiring changed under the caller: %+v", published["dev"])
	}
}

// A draft that listens to its own event closes a cycle of one.
func TestWiringWith_aDraftListeningToItself_closesACycleOfOne(t *testing.T) {
	t.Parallel()

	got := spec.WiringWith(nil, draft("dev", []string{"x"}, "x"))

	if cycles := domain.EventCyclesThrough(got, "dev").Cycles; !reflect.DeepEqual(cycles, []domain.EventCycle{{"dev"}}) {
		t.Errorf("cycles = %v, want dev → dev", cycles)
	}
}

// The edges a screen shows are drawn from the same wiring, the ones that go
// nowhere included, in a stable order.
func TestEdgesOf_drawsEveryEdgeIncludingTheOnesThatGoNowhere(t *testing.T) {
	t.Parallel()
	w := spec.WiringOf([]spec.Spec{
		draft("dev", []string{"code.ready", "nobody.listens"}, "nobody.emits"),
		draft("review", []string{"review.done"}, "code.ready"),
	})

	got := spec.EdgesOf(w)

	want := []domain.EventEdge{
		{From: "dev", Event: "code.ready", To: "review"},
		{From: "dev", Event: "nobody.listens"},
		{From: "review", Event: "review.done"},
		{Event: "nobody.emits", To: "dev"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("edges = %+v\nwant %+v", got, want)
	}
}
