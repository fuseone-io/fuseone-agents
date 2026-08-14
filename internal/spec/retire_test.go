package spec_test

import (
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/spec"
)

/*
Taking an agent out of circulation.

Deleting it is refused everywhere and always: a run is pinned to a version and
that version is the only correct explanation of what the run did. What was
missing is the honest alternative — an agent nobody uses any more that stops
appearing on every screen without taking its own history with it.
*/

func TestRetire_theAgentLeavesTheListingAndKeepsItsRuns(t *testing.T) {
	state, registry := retirable(t)
	ctx := t.Context()

	if err := state.Retire(ctx, "triage", "usr_ana"); err != nil {
		t.Fatalf("Retire: %v", err)
	}

	retired, err := state.Retired(ctx)
	if err != nil {
		t.Fatalf("Retired: %v", err)
	}
	if !retired["triage"] {
		t.Error("the agent is not recorded as retired")
	}

	// Still readable, whole. Somebody auditing a run from last year has to be
	// able to read the text that ran, and retiring is not erasing.
	versions, err := registry.Versions(ctx, "triage")
	if err != nil || len(versions) == 0 {
		t.Fatalf("versions = %v, err = %v; want the history intact", versions, err)
	}
}

// Retiring stops it acting. An agent taken out of circulation that a schedule
// still fires is worse than one nobody retired: the screen says it is gone.
func TestRetire_alsoStopsIt(t *testing.T) {
	state, _ := retirable(t)
	ctx := t.Context()

	if err := state.SetPaused(ctx, "triage", false, "usr_ana", now()); err != nil {
		t.Fatalf("SetPaused: %v", err)
	}
	if err := state.Retire(ctx, "triage", "usr_ana"); err != nil {
		t.Fatalf("Retire: %v", err)
	}

	paused, err := state.IsPaused(ctx, "triage")
	if err != nil {
		t.Fatalf("IsPaused: %v", err)
	}
	if !paused {
		t.Error("a retired agent is still running")
	}
}

func TestRestore_bringsItBackStopped(t *testing.T) {
	state, _ := retirable(t)
	ctx := t.Context()

	if err := state.Retire(ctx, "triage", "usr_ana"); err != nil {
		t.Fatalf("Retire: %v", err)
	}
	if err := state.Restore(ctx, "triage", "usr_ana"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	retired, _ := state.Retired(ctx)
	if retired["triage"] {
		t.Error("the agent is still retired")
	}
	// Stopped, not running. Bringing an agent back is not the same decision as
	// starting it, and doing both at once starts one nobody looked at.
	paused, _ := state.IsPaused(ctx, "triage")
	if !paused {
		t.Error("restoring started the agent")
	}
}

// --- harness ----------------------------------------------------------------

func now() time.Time { return time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC) }

// retirable publishes one agent and hands back the two halves of it: the state
// beside the specification, and the registry holding the specification itself.
func retirable(t *testing.T) (*spec.State, *spec.Registry) {
	t.Helper()
	registry := openRegistry(t)

	dsn := os.Getenv("TEST_DATABASE_URL")
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := registry.Publish(t.Context(), published(t, definition), "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return spec.NewState(pool), registry
}
