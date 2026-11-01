package ledger_test

import (
	"context"
	"testing"

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

		page, err := s.FinishedSince(ctx, domain.FinishCursor{}, 10)
		if err != nil || len(page) != 3 {
			t.Fatalf("FinishedSince = %v, %v", feedIDs(page), err)
		}
		if err := s.AdvanceEventCursor(ctx, page[0].At); err != nil {
			t.Fatalf("advance: %v", err)
		}

		got := backlogOf(t, s)
		if behind := got.Ready + got.HeldByXmin; behind != 2 {
			t.Fatalf("behind = %d (%+v), want the two real finished runs past the cursor", behind, got)
		}
		if want := page[1].Run.EndedAt; !got.OldestFinishedAt.Equal(want) {
			t.Fatalf("oldest finished at %v, want %v: the first run past the cursor", got.OldestFinishedAt, want)
		}
	})
}

func TestEventDispatchBacklog_caughtUp_isEmpty(t *testing.T) {
	run(t, "nothing waits once the cursor reached the last run", func(t *testing.T, s Store) {
		ctx := context.Background()
		finishRunIn(t, s, "run-a")
		page, err := s.FinishedSince(ctx, domain.FinishCursor{}, 10)
		if err != nil || len(page) != 1 {
			t.Fatalf("FinishedSince = %v, %v", feedIDs(page), err)
		}
		if err := s.AdvanceEventCursor(ctx, page[0].At); err != nil {
			t.Fatalf("advance: %v", err)
		}

		got := backlogOf(t, s)
		if got.Ready != 0 || got.HeldByXmin != 0 || !got.OldestFinishedAt.IsZero() {
			t.Fatalf("backlog = %+v, want empty when caught up", got)
		}
	})
}
