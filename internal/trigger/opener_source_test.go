package trigger_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

// A run opened from a recorded one (#16) chooses its version and names where
// its input came from. Both are facts sealed on the opening step, so neither
// can be supplied later by a reader that wants a different answer.

func historyOpener(t *testing.T) (*trigger.Opener, *ledger.Memory) {
	t.Helper()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	reg := registry{versions: []domain.AgentSummary{
		{ID: "triage", VersionID: "v2", Scope: scope, Latest: true},
		{ID: "triage", VersionID: "v1", Scope: scope},
	}}
	clock := fixedClock{t: time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)}
	return trigger.NewOpener(store, reg, clock).WithContent(engine.NewMemoryContent()), store
}

func TestOpen_explicitVersion_pinsTheRunToThatVersion(t *testing.T) {
	t.Parallel()
	opener, store := historyOpener(t)

	got, err := opener.Open(t.Context(), trigger.Request{
		Agent: "triage", IdemKey: "intent-v1", Trigger: "manual", Version: "v1",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	steps, err := store.Read(context.Background(), got.RunID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if steps[0].VersionID != "v1" {
		t.Errorf("version = %q, want the one asked for", steps[0].VersionID)
	}
}

func TestOpen_versionTheAgentNeverPublished_isRefusedAndOpensNothing(t *testing.T) {
	t.Parallel()
	opener, store := historyOpener(t)

	_, err := opener.Open(t.Context(), trigger.Request{
		Agent: "triage", IdemKey: "intent-v9", Trigger: "manual", Version: "v9",
	})
	if !errors.Is(err, trigger.ErrUnknownVersion) {
		t.Fatalf("Open = %v, want ErrUnknownVersion", err)
	}

	runs, err := store.Runs(context.Background())
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("the ledger holds %d runs, want none", len(runs))
	}
}

func TestOpen_fromARecordedRun_namesTheSourceByIDOnTheOpeningStep(t *testing.T) {
	t.Parallel()
	opener, store := historyOpener(t)

	got, err := opener.Open(t.Context(), trigger.Request{
		Agent: "triage", IdemKey: "intent-src", Trigger: "channel",
		SourceRun: "run_triage_1_abcd",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	steps, err := store.Read(context.Background(), got.RunID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var payload domain.RunStartedPayload
	decode(t, steps[0].Payload, &payload)
	if payload.SourceRun != "run_triage_1_abcd" {
		t.Errorf("source run = %q, want the run the input was taken from", payload.SourceRun)
	}
}
