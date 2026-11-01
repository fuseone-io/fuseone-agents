package ledger_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
The event dispatch backlog (#228): the finished runs past the cursor, and when
the oldest of them finished. The split between what the feed can read now and
what xmin holds back is Postgres's alone (see event_lag_postgres_test.go);
the total and the oldest are the contract both ledgers keep.
*/

func backlogOf(t *testing.T, s Store) domain.EventDispatchBacklog {
	t.Helper()
	got, err := s.EventDispatchBacklog(context.Background())
	if err != nil {
		t.Fatalf("EventDispatchBacklog: %v", err)
	}
	return got
}

func TestEventDispatchBacklog_countsTheRealFinishedRunsPastTheCursor(t *testing.T) {
	run(t, "the runs past the cursor, and the oldest of them", func(t *testing.T, s Store) {
		ctx := context.Background()
		for _, id := range []domain.RunID{"run-a", "run-b", "run-c"} {
			finishRunIn(t, s, id)
		}
		mustAppend(t, s, step("run-open", domain.StepRunStarted))
		sim := step("run-sim", domain.StepRunStarted)
		sim.Payload = []byte(`{"trigger":"manual","simulated":true,"simulation":"batch-1"}`)
		mustAppend(t, s, sim)
		mustAppend(t, s, step("run-sim", domain.StepRunFinished))
		if err := s.AdvanceEventCursor(ctx, positionOf(t, s, "run-a")); err != nil {
			t.Fatalf("advance: %v", err)
		}

		got := backlogOf(t, s)
		if behind := got.Ready + got.HeldByXmin; behind != 2 {
			t.Fatalf("behind = %d (%+v), want the two real finished runs past the cursor", behind, got)
		}
		if want := endedAt(t, s, "run-b"); !got.OldestFinishedAt.Equal(want) {
			t.Fatalf("oldest finished at %v, want %v: the first run past the cursor", got.OldestFinishedAt, want)
		}
	})
}

// endedAt is when a run ended, as the run listing reports it.
func endedAt(t *testing.T, s Store, id domain.RunID) time.Time {
	t.Helper()
	runs, err := s.ListRuns(context.Background(), domain.RunFilter{}, "", 100)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	for _, r := range runs {
		if r.RunID == id {
			return r.EndedAt
		}
	}
	t.Fatalf("%s is not listed", id)
	return time.Time{}
}

func TestEventDispatchBacklog_caughtUp_isEmpty(t *testing.T) {
	run(t, "nothing waits once the cursor reached the last run", func(t *testing.T, s Store) {
		finishRunIn(t, s, "run-a")
		if err := s.AdvanceEventCursor(context.Background(), positionOf(t, s, "run-a")); err != nil {
			t.Fatalf("advance: %v", err)
		}

		got := backlogOf(t, s)
		if got.Ready != 0 || got.HeldByXmin != 0 || !got.OldestFinishedAt.IsZero() {
			t.Fatalf("backlog = %+v, want empty when caught up", got)
		}
	})
}

/*
The runtime view carries the backlog for a caller who reads the installation,
and nothing for one who reads an area: the cursor is the installation's, and
its count is every scope's runs.
*/
func TestRuntimeHealth_carriesTheEventDispatchBacklogOnlyInstallationWide(t *testing.T) {
	run(t, "the backlog is an installation-wide fact", func(t *testing.T, s Store) {
		ctx := context.Background()
		finishRunIn(t, s, "run-a")

		wide, err := s.RuntimeHealth(ctx, domain.RunFilter{Scopes: []domain.Scope{{Company: domain.Installation}}})
		if err != nil {
			t.Fatalf("RuntimeHealth: %v", err)
		}
		if wide.EventDispatch == nil || wide.EventDispatch.Ready+wide.EventDispatch.HeldByXmin != 1 {
			t.Fatalf("installation-wide backlog = %+v, want the one finished run", wide.EventDispatch)
		}

		area, err := s.RuntimeHealth(ctx, domain.RunFilter{Scopes: []domain.Scope{{Company: "acme", Area: "cx"}}})
		if err != nil {
			t.Fatalf("RuntimeHealth: %v", err)
		}
		if area.EventDispatch != nil {
			t.Fatalf("area-scoped backlog = %+v, want none", area.EventDispatch)
		}
	})
}
