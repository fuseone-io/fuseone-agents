package ledger_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
The finished-run feed the event dispatcher reads (#203): real finished runs
in the order their finishing transactions were allocated ids — read only
below pg_snapshot_xmin, where that order is stable — after a cursor that only
moves forward. A run that finishes between two sweeps, or a burst larger
than one page, is read by a later page rather than left behind.
*/

func finishRunIn(t *testing.T, s Store, id domain.RunID) {
	t.Helper()
	mustAppend(t, s, step(id, domain.StepRunStarted))
	mustAppend(t, s, step(id, domain.StepRunFinished))
}

func feedIDs(runs []domain.FinishedRun) []domain.RunID {
	out := make([]domain.RunID, len(runs))
	for i, r := range runs {
		out[i] = r.Run.RunID
	}
	return out
}

func TestFinishedSince_pagesFinishedRunsInFinishingOrderAfterTheCursor(t *testing.T) {
	run(t, "finished runs in the order they finished", func(t *testing.T, s Store) {
		var want []domain.FinishCursor
		for _, id := range []domain.RunID{"run-c", "run-a", "run-b"} {
			finishRunIn(t, s, id)
			want = append(want, positionOf(t, s, id))
		}
		mustAppend(t, s, step("run-open", domain.StepRunStarted))
		settle(t, s, want...)

		// The order they finished in; a run still going is never offered.
		first, w := readFeed(t, s, domain.FinishCursor{}, 2)
		requireFeedPrefix(t, first, w, want, 2)
		if len(first) < 2 {
			return
		}
		second, w := readFeed(t, s, first[1].At, 2)
		requireFeedPrefix(t, second, w, want[2:], 2)
		if len(second) == 1 && !first[1].At.Before(second[0].At) {
			t.Fatalf("cursor %+v is not before %+v", first[1].At, second[0].At)
		}
	})
}

func TestFinishedSince_leavesSimulatedRunsOut(t *testing.T) {
	run(t, "simulated runs are not offered", func(t *testing.T, s Store) {
		started := step("run-sim", domain.StepRunStarted)
		started.Payload = []byte(`{"trigger":"manual","simulated":true,"simulation":"batch-1"}`)
		mustAppend(t, s, started)
		mustAppend(t, s, step("run-sim", domain.StepRunFinished))

		got, err := s.FinishedSince(context.Background(), domain.FinishCursor{}, 10)
		if err != nil || len(got) != 0 {
			t.Fatalf("FinishedSince = %v, %v; a simulation publishes nothing", feedIDs(got), err)
		}
	})
}

func TestAdvanceEventCursor_onlyMovesForward(t *testing.T) {
	run(t, "the cursor never moves back", func(t *testing.T, s Store) {
		ctx := context.Background()
		finishRunIn(t, s, "run-a")
		finishRunIn(t, s, "run-b")
		a, b := positionOf(t, s, "run-a"), positionOf(t, s, "run-b")

		if err := s.AdvanceEventCursor(ctx, b); err != nil {
			t.Fatalf("advance: %v", err)
		}
		// A replica that finished an earlier page later must not move it back.
		if err := s.AdvanceEventCursor(ctx, a); err != nil {
			t.Fatalf("advance back: %v", err)
		}
		got, err := s.EventCursor(ctx)
		if err != nil {
			t.Fatalf("EventCursor: %v", err)
		}
		if got != b {
			t.Fatalf("cursor = %+v, want the furthest %+v", got, b)
		}
	})
}
