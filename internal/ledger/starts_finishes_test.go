package ledger_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
The event dispatcher reads, for a batch of finished runs, only what a
dispatch needs: each run's opening step (what opened it, its chain) and its
closing step (what it published). One read for the batch, not every step of
every run (#203).

A run is addressed by its id and the moment it opened — the partition key of
run_steps — so the read touches the months the batch opened in, not every
partition the ledger has.
*/

// asked is the summaries of the runs that exist among ids, as the dispatcher
// holds them.
func asked(t *testing.T, s Store, ids ...domain.RunID) []domain.RunSummary {
	t.Helper()
	all, err := s.ListRuns(context.Background(), domain.RunFilter{}, "", 100)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	byID := map[domain.RunID]domain.RunSummary{}
	for _, r := range all {
		byID[r.RunID] = r
	}
	var out []domain.RunSummary
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		} else {
			out = append(out, domain.RunSummary{RunID: id, StartedAt: time.Now()})
		}
	}
	return out
}

func TestStartsAndFinishes_returnsOnlyTheOpeningAndClosingStepsOfTheAskedRuns(t *testing.T) {
	run(t, "starts and finishes of a batch", func(t *testing.T, s Store) {
		for _, id := range []domain.RunID{"run-a", "run-b", "run-other"} {
			mustAppend(t, s, step(id, domain.StepRunStarted))
			mustAppend(t, s, step(id, domain.StepPlanned))
			mustAppend(t, s, step(id, domain.StepRunFinished))
		}
		mustAppend(t, s, step("run-open", domain.StepRunStarted))
		mustAppend(t, s, step("run-open", domain.StepPlanned))

		got, err := s.StartsAndFinishes(context.Background(), asked(t, s, "run-a", "run-b", "run-open", "run-unknown"))
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

// The opening moment is part of a run's address: asked with another, the run
// is not found. That is what lets Postgres read one partition per month the
// batch opened in rather than all of them.
func TestStartsAndFinishes_aRunAskedWithAnotherOpeningMoment_isNotFound(t *testing.T) {
	run(t, "the opening moment addresses the run", func(t *testing.T, s Store) {
		mustAppend(t, s, step("run-a", domain.StepRunStarted))
		mustAppend(t, s, step("run-a", domain.StepRunFinished))
		wrong := asked(t, s, "run-a")
		wrong[0].StartedAt = wrong[0].StartedAt.Add(-48 * time.Hour)

		got, err := s.StartsAndFinishes(context.Background(), wrong)
		if err != nil || len(got) != 0 {
			t.Fatalf("StartsAndFinishes = %v, %v; want nothing for another opening moment", got, err)
		}
	})
}

// Each id is matched with its own moment, not with any moment of the batch:
// two runs asked with each other's opening moments are not found.
func TestStartsAndFinishes_idsAndMomentsSwappedBetweenRuns_findNothing(t *testing.T) {
	run(t, "each id with its own moment", func(t *testing.T, s Store) {
		for i, id := range []domain.RunID{"run-a", "run-b"} {
			started := step(id, domain.StepRunStarted)
			started.At = time.Date(2026, 10, 1+i, 9, 0, 0, 0, time.UTC)
			mustAppend(t, s, started)
			mustAppend(t, s, step(id, domain.StepRunFinished))
		}
		swapped := asked(t, s, "run-a", "run-b")
		swapped[0].StartedAt, swapped[1].StartedAt = swapped[1].StartedAt, swapped[0].StartedAt

		got, err := s.StartsAndFinishes(context.Background(), swapped)
		if err != nil || len(got) != 0 {
			t.Fatalf("StartsAndFinishes = %d runs, %v; want nothing for swapped moments", len(got), err)
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
