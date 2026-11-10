package ledger_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
An agent's experiments, newest first (NT-015 § Experiments).

Derived from the runs, as batteries are: an experiment is exactly the runs that name it.
Only those runs: a battery replays the same cases under a simulation id, and a real run
names nothing, and listing either as an experiment would offer the console something to
compare that is not one. Bounded, because the console asks for a page and not a history.
*/

// openedExperiment is one run of an experiment of an agent, opened at a moment.
func openedExperiment(
	t *testing.T, run domain.RunID, agent domain.AgentID, experiment string, at time.Time,
) domain.Step {
	t.Helper()
	st := startedAt(run, at)
	st.AgentID, st.IdemKey = agent, string(run)
	st.Payload = mustJSON(t, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: experiment,
		Case: "case-1", DatasetVersion: 2, Experiment: experiment,
	})
	return st
}

func TestExperiments_listsOnlyTheAgentsExperimentsNewestFirst(t *testing.T) {
	base := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	run(t, "an agent's experiments, newest first", func(t *testing.T, s Store) {
		ctx := context.Background()
		mustAppend(t, s, openedExperiment(t, "run-old-1", "triage", "exp-old", base))
		// A later run of the old experiment does not make it newer: it opened first.
		mustAppend(t, s, openedExperiment(t, "run-old-2", "triage", "exp-old", base.Add(3*time.Hour)))
		mustAppend(t, s, openedExperiment(t, "run-new", "triage", "exp-new", base.Add(time.Hour)))
		mustAppend(t, s, openedExperiment(t, "run-other", "other", "exp-other", base.Add(2*time.Hour)))
		mustAppend(t, s, batteryRun(t, "run-battery", base.Add(4*time.Hour)))
		mustAppend(t, s, startedAt("run-real", base.Add(5*time.Hour)))

		got, err := s.Experiments(ctx, "triage", 10)
		if err != nil {
			t.Fatalf("Experiments: %v", err)
		}
		if len(got) != 2 || got[0] != "exp-new" || got[1] != "exp-old" {
			t.Errorf("experiments = %v, want exp-new then exp-old, and nothing else", got)
		}
	})
}

func TestExperiments_returnsNoMoreThanAsked(t *testing.T) {
	base := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	run(t, "bounded", func(t *testing.T, s Store) {
		ctx := context.Background()
		mustAppend(t, s, openedExperiment(t, "run-a", "triage", "exp-a", base))
		mustAppend(t, s, openedExperiment(t, "run-b", "triage", "exp-b", base.Add(time.Hour)))

		got, err := s.Experiments(ctx, "triage", 1)
		if err != nil || len(got) != 1 || got[0] != "exp-b" {
			t.Errorf("Experiments(limit 1) = %v err=%v, want only the newest", got, err)
		}
		if none, err := s.Experiments(ctx, "triage", 0); err != nil || len(none) != 0 {
			t.Errorf("Experiments(limit 0) = %v err=%v, want nothing", none, err)
		}
	})
}
