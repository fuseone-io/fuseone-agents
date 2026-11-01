package ledger_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
The event dispatcher reads, for a batch of finished runs, only what a
dispatch needs: each run's opening step (what opened it, its chain) and its
closing step (what it published). One read for the batch, not every step of
every run (#203).
*/

func TestStartsAndFinishes_returnsOnlyTheOpeningAndClosingStepsOfTheAskedRuns(t *testing.T) {
	run(t, "starts and finishes of a batch", func(t *testing.T, s Store) {
		for _, id := range []domain.RunID{"run-a", "run-b", "run-other"} {
			mustAppend(t, s, step(id, domain.StepRunStarted))
			mustAppend(t, s, step(id, domain.StepPlanned))
			mustAppend(t, s, step(id, domain.StepRunFinished))
		}
		mustAppend(t, s, step("run-open", domain.StepRunStarted))
		mustAppend(t, s, step("run-open", domain.StepPlanned))

		got, err := s.StartsAndFinishes(context.Background(), []domain.RunID{"run-a", "run-b", "run-open", "run-unknown"})
		if err != nil {
			t.Fatalf("StartsAndFinishes: %v", err)
		}

		want := map[domain.RunID][]domain.StepKind{
			"run-a":    {domain.StepRunStarted, domain.StepRunFinished},
			"run-b":    {domain.StepRunStarted, domain.StepRunFinished},
			"run-open": {domain.StepRunStarted},
		}
		if len(got) != len(want) {
			t.Fatalf("runs = %d, want %d: only the asked runs that exist", len(got), len(want))
		}
		for id, kinds := range want {
			steps := got[id]
			if len(steps) != len(kinds) {
				t.Fatalf("%s: %d steps, want %v", id, len(steps), kinds)
			}
			for i, kind := range kinds {
				if steps[i].Kind != kind || steps[i].RunID != id {
					t.Fatalf("%s step %d = %s of %s, want %s", id, i, steps[i].Kind, steps[i].RunID, kind)
				}
			}
		}
	})
}

func TestStartsAndFinishes_anEmptyBatch_readsNothing(t *testing.T) {
	run(t, "an empty batch", func(t *testing.T, s Store) {
		mustAppend(t, s, step("run-a", domain.StepRunStarted))

		got, err := s.StartsAndFinishes(context.Background(), nil)
		if err != nil || len(got) != 0 {
			t.Fatalf("StartsAndFinishes(nil) = %v, %v; want nothing", got, err)
		}
	})
}
