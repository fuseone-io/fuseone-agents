package trigger_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

// What makes a scheduler safe is not that it fires — anything fires. It is
// that a moment fires once, that a moment nobody was awake for is not
// replayed, and that a schedule nobody can parse does not jam the rest.

var noon = time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

// schedules is an in-memory Schedules whose rows a test can inspect.
type schedules struct {
	due      []trigger.Due
	advanced map[string]time.Time
}

func (s *schedules) Due(context.Context, time.Time) ([]trigger.Due, error) {
	return s.due, nil
}

func (s *schedules) Advance(_ context.Context, agent domain.AgentID, schedule string, next time.Time) error {
	if s.advanced == nil {
		s.advanced = map[string]time.Time{}
	}
	s.advanced[string(agent)+"|"+schedule] = next
	return nil
}

func (s *schedules) Sync(context.Context, domain.AgentID, []string, time.Time) error { return nil }

func schedulerFor(t *testing.T, due []trigger.Due, now time.Time) (*trigger.Scheduler, *ledger.Memory, *schedules) {
	t.Helper()
	store := ledger.NewMemory()
	reg := registry{versions: []domain.AgentSummary{{
		ID: "triage", VersionID: "v2", Scope: domain.Scope{Company: "acme", Area: "cx"}, Latest: true,
	}}}
	clock := fixedClock{t: now}
	rows := &schedules{due: due}
	opener := trigger.NewOpener(store, reg, clock)

	return trigger.NewScheduler(rows, opener, clock, slog.New(slog.NewTextHandler(io.Discard, nil))), store, rows
}

func TestTick_dueSchedule_opensARun(t *testing.T) {
	t.Parallel()

	s, store, _ := schedulerFor(t, []trigger.Due{
		{Agent: "triage", Schedule: "*/15 * * * *", At: noon},
	}, noon)

	opened, err := s.Tick(t.Context())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if opened != 1 {
		t.Fatalf("opened = %d, want 1", opened)
	}

	runs, _ := store.Runs(context.Background())
	if len(runs) != 1 {
		t.Errorf("the ledger holds %d runs, want 1", len(runs))
	}
}

func TestTick_twoWorkersOnTheSameMoment_openOneRunBetweenThem(t *testing.T) {
	t.Parallel()

	// The property the whole design rests on. No lease, no lock: both workers
	// name the moment identically, and the ledger accepts one of them.
	store := ledger.NewMemory()
	reg := registry{versions: []domain.AgentSummary{{
		ID: "triage", VersionID: "v2", Scope: domain.Scope{Company: "acme", Area: "cx"},
	}}}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	due := []trigger.Due{{Agent: "triage", Schedule: "*/15 * * * *", At: noon}}

	var opened int
	for range 2 {
		worker := trigger.NewScheduler(
			&schedules{due: due},
			trigger.NewOpener(store, reg, fixedClock{t: noon}),
			fixedClock{t: noon}, quiet,
		)
		n, err := worker.Tick(t.Context())
		if err != nil {
			t.Fatalf("Tick: %v", err)
		}
		opened += n
	}

	if opened != 1 {
		t.Errorf("the two workers opened %d runs between them, want 1", opened)
	}
	runs, _ := store.Runs(context.Background())
	if len(runs) != 1 {
		t.Errorf("the ledger holds %d runs, want 1", len(runs))
	}
}

func TestTick_momentThatPassedWhileNothingRan_isNotReplayed(t *testing.T) {
	t.Parallel()

	// An hour of backlog emptied into a system that has just come back up is
	// worse than a missed hour, and firing the newest missed slot at start-up
	// would mean deploying the platform runs every scheduled agent at once.
	late := noon.Add(trigger.Grace + time.Minute)
	s, store, _ := schedulerFor(t, []trigger.Due{
		{Agent: "triage", Schedule: "*/15 * * * *", At: noon},
	}, late)

	opened, err := s.Tick(t.Context())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if opened != 0 {
		t.Errorf("opened = %d, want the stale moment skipped", opened)
	}

	runs, _ := store.Runs(context.Background())
	if len(runs) != 0 {
		t.Errorf("the ledger holds %d runs, want none", len(runs))
	}
}

func TestTick_staleMoment_stillAdvancesSoItDoesNotJam(t *testing.T) {
	t.Parallel()

	// Skipping without advancing would read as due on every tick forever.
	late := noon.Add(time.Hour)
	s, _, rows := schedulerFor(t, []trigger.Due{
		{Agent: "triage", Schedule: "*/15 * * * *", At: noon},
	}, late)

	if _, err := s.Tick(t.Context()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	next, moved := rows.advanced["triage|*/15 * * * *"]
	if !moved || !next.After(late) {
		t.Errorf("next = %v (moved=%v), want a moment after now", next, moved)
	}
}

func TestTick_unparseableSchedule_doesNotStopTheOthers(t *testing.T) {
	t.Parallel()

	s, store, _ := schedulerFor(t, []trigger.Due{
		{Agent: "triage", Schedule: "every other tuesday", At: noon},
		{Agent: "triage", Schedule: "*/15 * * * *", At: noon},
	}, noon)

	opened, err := s.Tick(t.Context())
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if opened != 1 {
		t.Errorf("opened = %d, want the parseable schedule to have fired", opened)
	}
	runs, _ := store.Runs(context.Background())
	if len(runs) != 1 {
		t.Errorf("the ledger holds %d runs, want 1", len(runs))
	}
}

func TestNextAfter_returnsTheNextMomentStrictlyAfterNow(t *testing.T) {
	t.Parallel()

	// Strictly after: returning the current moment would make a tick fire the
	// same slot it just handled.
	got, err := trigger.NextAfter("*/15 * * * *", noon)
	if err != nil {
		t.Fatalf("NextAfter: %v", err)
	}
	if want := noon.Add(15 * time.Minute); !got.Equal(want) {
		t.Errorf("next = %s, want %s", got, want)
	}
}

func TestValidSchedule_refusesASecondsField(t *testing.T) {
	t.Parallel()

	// A schedule finer than a minute is a queue, and this is not one. Better
	// refused at publication than found by an operator wondering why an agent
	// runs sixty times an hour.
	if err := trigger.ValidSchedule("*/30 * * * * *"); err == nil {
		t.Fatal("a six-field schedule was accepted")
	}
	if err := trigger.ValidSchedule("@hourly"); err != nil {
		t.Errorf("@hourly was refused: %v", err)
	}
}
