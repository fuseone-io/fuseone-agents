package trigger_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

// Opening a run is the one act on this platform that reaches the real world.
// Everything here is about it happening exactly as often as somebody meant it.

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

type registry struct{ versions []domain.AgentSummary }

func (r registry) Versions(context.Context, domain.AgentID) ([]domain.AgentSummary, error) {
	return r.versions, nil
}

func openerFor(t *testing.T) (*trigger.Opener, *ledger.Memory) {
	t.Helper()
	store := ledger.NewMemory()
	reg := registry{versions: []domain.AgentSummary{{
		ID: "triage", VersionID: "v2", Scope: domain.Scope{Company: "acme", Area: "cx"}, Latest: true,
	}}}
	clock := fixedClock{t: time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)}
	return trigger.NewOpener(store, reg, clock).WithContent(engine.NewMemoryContent()), store
}

func TestOpen_pinsTheRunToTheVersionPublishedNow(t *testing.T) {
	t.Parallel()
	opener, store := openerFor(t)

	got, err := opener.Open(t.Context(), trigger.Request{
		Agent: "triage", IdemKey: "intent-1", Trigger: "cron",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !got.Created {
		t.Error("the first call did not open the run")
	}

	steps, err := store.Read(context.Background(), got.RunID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// Pinned: a version published a second later never changes this run.
	if steps[0].VersionID != "v2" {
		t.Errorf("version = %q, want the one published when it opened", steps[0].VersionID)
	}
}

func TestOpen_sameKeyTwice_namesTheSameRunAndOpensOnlyOne(t *testing.T) {
	t.Parallel()
	opener, store := openerFor(t)

	first, err := opener.Open(t.Context(), trigger.Request{Agent: "triage", IdemKey: "intent-1"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	second, err := opener.Open(t.Context(), trigger.Request{Agent: "triage", IdemKey: "intent-1"})
	if err != nil {
		t.Fatalf("Open again: %v", err)
	}

	if second.RunID != first.RunID {
		t.Errorf("second call named %q, want %q", second.RunID, first.RunID)
	}
	if second.Created {
		t.Error("the second call reported opening a run that already existed")
	}

	runs, err := store.Runs(context.Background())
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("the ledger holds %d runs, want 1", len(runs))
	}
}

func TestOpen_withoutAKey_isRefused(t *testing.T) {
	t.Parallel()
	opener, _ := openerFor(t)

	// Without a key there is no way to tell a retry from a second intention,
	// and the safe reading of that ambiguity is to refuse rather than to run
	// something twice.
	if _, err := opener.Open(t.Context(), trigger.Request{Agent: "triage"}); err == nil {
		t.Fatal("a run opened with no idempotency key")
	}
}

func TestOpen_recordsWhatCausedTheRun(t *testing.T) {
	t.Parallel()
	opener, store := openerFor(t)

	got, err := opener.Open(t.Context(), trigger.Request{
		Agent: "triage", IdemKey: "intent-1", Trigger: "cron",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	steps, _ := store.Read(context.Background(), got.RunID, domain.FirstSeq)
	var payload domain.RunStartedPayload
	decode(t, steps[0].Payload, &payload)

	// "Why did this run" is the first question asked about any run nobody
	// remembers starting.
	if payload.Trigger != "cron" {
		t.Errorf("trigger = %q, want cron", payload.Trigger)
	}
}

func TestOpen_unpublishedAgent_isRefused(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	opener := trigger.NewOpener(store, registry{}, fixedClock{t: time.Now()})

	if _, err := opener.Open(t.Context(), trigger.Request{Agent: "ghost", IdemKey: "k"}); err == nil {
		t.Fatal("a run opened for an agent nobody published")
	}
}

func decode(t *testing.T, raw []byte, into any) {
	t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
}

func TestOpen_pausedAgent_opensNothing(t *testing.T) {
	t.Parallel()
	opener, store := openerFor(t)
	opener = opener.WithPauses(paused{"triage": true})

	// Every way a run can start goes through here, which is the point of the
	// Opener existing. A pause honoured by the scheduler and not by the
	// webhook would be a pause that stops an agent on weekdays.
	if _, err := opener.Open(t.Context(), trigger.Request{
		Agent: "triage", IdemKey: "intent-1", Trigger: "cron",
	}); !errors.Is(err, trigger.ErrPaused) {
		t.Fatalf("Open on a paused agent = %v, want ErrPaused", err)
	}

	runs, _ := store.Runs(context.Background())
	if len(runs) != 0 {
		t.Errorf("the ledger holds %d runs, want none", len(runs))
	}
}

func TestOpen_agentNobodyStarted_isTreatedAsPaused(t *testing.T) {
	t.Parallel()
	opener, _ := openerFor(t)
	opener = opener.WithPauses(paused{})

	// A new agent is created paused, so an absent row means nobody ever
	// decided otherwise. Reading that as running would let an agent start
	// because a write failed.
	if _, err := opener.Open(t.Context(), trigger.Request{
		Agent: "triage", IdemKey: "intent-1",
	}); !errors.Is(err, trigger.ErrPaused) {
		t.Fatalf("Open for an agent with no state = %v, want ErrPaused", err)
	}
}

func TestOpen_runningAgent_opensAsBefore(t *testing.T) {
	t.Parallel()
	opener, _ := openerFor(t)
	opener = opener.WithPauses(paused{"triage": false})

	if _, err := opener.Open(t.Context(), trigger.Request{
		Agent: "triage", IdemKey: "intent-1",
	}); err != nil {
		t.Fatalf("Open on a running agent: %v", err)
	}
}

// paused stands in for the state store.
type paused map[domain.AgentID]bool

func (p paused) IsPaused(_ context.Context, agent domain.AgentID) (bool, error) {
	stopped, known := p[agent]
	return !known || stopped, nil
}

func TestOpen_twoIntentionsAtTheSameInstant_openSeparateRuns(t *testing.T) {
	t.Parallel()
	opener, _ := openerFor(t)

	first, err := opener.Open(t.Context(), trigger.Request{
		Agent: "triage", IdemKey: "intent-1", Trigger: "webhook",
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := opener.Open(t.Context(), trigger.Request{
		Agent: "triage", IdemKey: "intent-2", Trigger: "webhook",
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	// Two webhooks arriving in the same millisecond are two intentions. An id
	// derived from the clock alone gives them one run, and the second loses a
	// sequence conflict on a step it had every right to append.
	if second.RunID == first.RunID {
		t.Fatalf("both intentions opened %q", first.RunID)
	}
}
