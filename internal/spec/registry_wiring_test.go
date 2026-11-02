package spec_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/spec"
)

/*
The graph is drawn from each agent's current version, read in one query.

The current version is the one somebody chose, else the newest: a rollback
must change the graph back, or a judgement made after it would be made
against a version that no longer runs.
*/
func TestEdges_drawFromTheCurrentVersion_theChosenOneOverTheNewest(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	registry := spec.NewRegistry(pool)
	older := looping(t, "developer", "older.event", "review.done")
	newer := looping(t, "developer", "newer.event", "review.done")
	for _, s := range []spec.Spec{older, newer} {
		if err := registry.Publish(ctx, s, "usr_ana", "acme"); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	if emitted(t, registry) != "newer.event" {
		t.Fatalf("with nothing chosen, the graph should be the newest version's")
	}

	if err := registry.MakeCurrent(ctx, older.ID, older.Version); err != nil {
		t.Fatalf("MakeCurrent: %v", err)
	}

	if got := emitted(t, registry); got != "older.event" {
		t.Errorf("after the rollback the graph emits %q, want older.event", got)
	}
}

// emitted is the one event the graph shows developer publishing.
func emitted(t *testing.T, registry *spec.Registry) string {
	t.Helper()
	edges, err := registry.Edges(context.Background())
	if err != nil {
		t.Fatalf("Edges: %v", err)
	}
	for _, e := range edges {
		if e.From == "developer" {
			return e.Event
		}
	}
	return ""
}
