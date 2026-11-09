package ledger_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
An experiment is never a battery, and never the simulation that lets an agent leave Draft
(NT-015 § Experiments, rule 1).

An experiment replays the cases of a dataset version exactly as a battery does: simulated,
under a simulation id, naming each case. What sets it apart is the experiment id alone, so
that is what every door has to read. Each door is checked with the two configurations an
adversary would bring: none at all, which is the run most likely to read as a battery, and
another model, which is the version as it would never run.

The doors are the three readers the start gate, the drift sweep and FU-10 decide on, on
both stores. A fake that let an experiment through would certify a gate production does
not have; one that refused what production accepts would hide that production lets it in.
*/

var experimentConfigs = []struct {
	name   string
	config *domain.ExperimentConfig
}{
	{"an empty configuration", nil},
	{"another model", &domain.ExperimentConfig{Provider: "openai", Model: "gpt-5.5"}},
}

// experimentRun opens one case of an experiment: everything a battery run carries, and an
// experiment id beside it.
func experimentRun(
	t *testing.T, id domain.RunID, at time.Time, config *domain.ExperimentConfig,
) domain.Step {
	t.Helper()
	st := startedAt(id, at)
	st.IdemKey = string(id)
	st.Payload = mustJSON(t, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "sim-experiment",
		Case: "case-1", DatasetVersion: 2,
		Experiment: "exp-1", ExperimentConfig: config,
	})
	return st
}

// batteryRun opens one case of a battery against the same version.
func batteryRun(t *testing.T, id domain.RunID, at time.Time) domain.Step {
	t.Helper()
	st := startedAt(id, at)
	st.IdemKey = string(id)
	st.Payload = mustJSON(t, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "sim-battery",
		Case: "case-1", DatasetVersion: 2,
	})
	return st
}

func TestBatteries_experiment_isNeverABattery(t *testing.T) {
	base := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	for _, c := range experimentConfigs {
		run(t, c.name, func(t *testing.T, s Store) {
			ctx := context.Background()
			mustAppend(t, s, batteryRun(t, "run-battery", base))
			// Newer than the battery, so a door that let it in would put it first.
			mustAppend(t, s, experimentRun(t, "run-experiment", base.Add(time.Hour), c.config))

			got, err := s.Batteries(ctx, "triage", "v3", 10)
			if err != nil {
				t.Fatalf("Batteries: %v", err)
			}
			if len(got) != 1 || got[0] != "sim-battery" {
				t.Errorf("batteries = %v, want only the battery", got)
			}
		})
	}
}

func TestLastBatteryAt_experiment_doesNotResetTheClock(t *testing.T) {
	base := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	for _, c := range experimentConfigs {
		run(t, c.name+", after a battery", func(t *testing.T, s Store) {
			mustAppend(t, s, batteryRun(t, "run-battery", base))
			mustAppend(t, s, experimentRun(t, "run-experiment", base.Add(time.Hour), c.config))

			at, ran, err := s.LastBatteryAt(context.Background(), "triage", "v3")
			if err != nil || !ran {
				t.Fatalf("LastBatteryAt: %v ran=%v err=%v", at, ran, err)
			}
			if !at.Equal(base) {
				t.Errorf("at = %s, want the battery's, not the experiment's", at)
			}
		})
		run(t, c.name+", alone", func(t *testing.T, s Store) {
			mustAppend(t, s, experimentRun(t, "run-experiment", base, c.config))

			at, ran, err := s.LastBatteryAt(context.Background(), "triage", "v3")
			if err != nil || ran {
				t.Errorf("LastBatteryAt = %v ran=%v err=%v, want no battery", at, ran, err)
			}
		})
	}
}

func TestHasSimulation_experiment_doesNotLetAnAgentLeaveDraft(t *testing.T) {
	base := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	for _, c := range experimentConfigs {
		run(t, c.name, func(t *testing.T, s Store) {
			mustAppend(t, s, experimentRun(t, "run-experiment", base, c.config))

			simulated, err := s.HasSimulation(context.Background(), "triage")
			if err != nil || simulated {
				t.Errorf("HasSimulation = %v err=%v, want false: the agent was never simulated as it is",
					simulated, err)
			}
		})
	}
	// The anchor: the same store answers yes for the simulation that does count, so the
	// no above is the exclusion and not a reader that never finds anything.
	run(t, "a battery beside it still counts", func(t *testing.T, s Store) {
		mustAppend(t, s, experimentRun(t, "run-experiment", base, nil))
		mustAppend(t, s, batteryRun(t, "run-battery", base.Add(time.Hour)))

		simulated, err := s.HasSimulation(context.Background(), "triage")
		if err != nil || !simulated {
			t.Errorf("HasSimulation = %v err=%v, want true", simulated, err)
		}
	})
}
