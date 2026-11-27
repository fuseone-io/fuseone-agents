package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// A server only a person can discover, observed with nobody connected, waits
// for a person (#483, delivery 3b). The observation is stored with the server
// configuration it was made under, so a reader can tell whether it is still
// about the server as configured now.

func awaitingObservation(name string, generation time.Time) domain.IntegrationHealth {
	return domain.IntegrationHealth{
		Name: name, Reachable: false, Detail: "nobody connected",
		ObservedAt: generation.Add(time.Minute), ObservedBy: "worker-a",
		AwaitingPerson: true, AwaitingGeneration: generation,
	}
}

func TestRecord_awaitingAPerson_keepsTheGenerationItWasObservedUnder(t *testing.T) {
	health := newHealth(t)
	ctx := context.Background()
	generation := time.Date(2026, 11, 27, 9, 0, 0, 123456000, time.UTC)

	if err := health.Record(ctx, awaitingObservation("dpg", generation)); err != nil {
		t.Fatalf("Record: %v", err)
	}

	all, err := health.All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	got := all["dpg"]
	if !got.AwaitingPerson || !got.AwaitingGeneration.Equal(generation) {
		t.Fatalf("health = %#v, want awaiting a person under generation %s", got, generation)
	}
}

func TestRecord_anyOtherObservation_clearsAwaitingAPerson(t *testing.T) {
	cases := map[string]domain.IntegrationHealth{
		"reached":         {Name: "dpg", Reachable: true, ToolCount: 4},
		"another failure": {Name: "dpg", Reachable: false, Detail: "connection refused"},
	}
	for name, next := range cases {
		t.Run(name, func(t *testing.T) {
			health := newHealth(t)
			ctx := context.Background()
			generation := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
			if err := health.Record(ctx, awaitingObservation("dpg", generation)); err != nil {
				t.Fatalf("Record awaiting: %v", err)
			}
			next.ObservedAt, next.ObservedBy = generation.Add(time.Hour), "worker-b"
			if err := health.Record(ctx, next); err != nil {
				t.Fatalf("Record next: %v", err)
			}

			got, found, err := health.Of(ctx, "dpg")
			if err != nil || !found {
				t.Fatalf("Of = %v, %v", found, err)
			}
			if got.AwaitingPerson || !got.AwaitingGeneration.IsZero() {
				t.Fatalf("health = %#v, want the wait over once something else was observed", got)
			}
		})
	}
}

func TestOf_readsOneServersObservation(t *testing.T) {
	health := newHealth(t)
	ctx := context.Background()
	generation := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	if err := health.Record(ctx, awaitingObservation("dpg", generation)); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := health.Record(ctx, domain.IntegrationHealth{
		Name: "crm", Reachable: true, ObservedAt: generation, ObservedBy: "worker-a",
	}); err != nil {
		t.Fatalf("Record crm: %v", err)
	}

	got, found, err := health.Of(ctx, "dpg")
	if err != nil || !found {
		t.Fatalf("Of(dpg) = %v, %v", found, err)
	}
	if got.Name != "dpg" || !got.AwaitingPerson {
		t.Errorf("Of(dpg) = %#v, want dpg's own observation", got)
	}
	if _, found, err := health.Of(ctx, "never"); err != nil || found {
		t.Errorf("Of(never) = %v, %v, want not found and no error", found, err)
	}
}
