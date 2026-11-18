package ledger_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
A rehearsal is never a battery, and never the simulation that lets an agent leave Draft
(NT-018 § A rehearsal is not a battery).

A rehearsal is simulated, names a simulation and a case, and names no experiment: exactly
what the three readers count. The rehearsal mark alone sets it apart, so each door is
checked with a run that carries everything a battery run does and the mark beside it. The
readers ship one release before anything writes a rehearsal, because an older replica
would count one.
*/

func rehearsalRun(t *testing.T, id domain.RunID, at time.Time) domain.Step {
	t.Helper()
	st := startedAt(id, at)
	st.IdemKey = string(id)
	st.Payload = mustJSON(t, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "sim-rehearsal",
		Case: "case-1", DatasetVersion: 2, Rehearsal: true,
	})
	return st
}

func TestBatteries_rehearsal_isNeverABattery(t *testing.T) {
	base := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	run(t, "newer than a battery", func(t *testing.T, s Store) {
		mustAppend(t, s, batteryRun(t, "run-battery", base))
		mustAppend(t, s, rehearsalRun(t, "run-rehearsal", base.Add(time.Hour)))

		got, err := s.Batteries(context.Background(), "triage", "v3", 10)
		if err != nil {
			t.Fatalf("Batteries: %v", err)
		}
		if len(got) != 1 || got[0] != "sim-battery" {
			t.Errorf("batteries = %v, want only the battery", got)
		}
	})
}

func TestLastBatteryAt_rehearsal_doesNotResetTheClock(t *testing.T) {
	base := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	run(t, "after a battery", func(t *testing.T, s Store) {
		mustAppend(t, s, batteryRun(t, "run-battery", base))
		mustAppend(t, s, rehearsalRun(t, "run-rehearsal", base.Add(time.Hour)))

		at, ran, err := s.LastBatteryAt(context.Background(), "triage", "v3")
		if err != nil || !ran {
			t.Fatalf("LastBatteryAt: %v ran=%v err=%v", at, ran, err)
		}
		if !at.Equal(base) {
			t.Errorf("at = %s, want the battery's, not the rehearsal's", at)
		}
	})
	run(t, "alone", func(t *testing.T, s Store) {
		mustAppend(t, s, rehearsalRun(t, "run-rehearsal", base))

		at, ran, err := s.LastBatteryAt(context.Background(), "triage", "v3")
		if err != nil || ran {
			t.Errorf("LastBatteryAt = %v ran=%v err=%v, want no battery", at, ran, err)
		}
	})
}

func TestHasSimulation_rehearsal_doesNotLetAnAgentLeaveDraft(t *testing.T) {
	base := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	run(t, "alone", func(t *testing.T, s Store) {
		mustAppend(t, s, rehearsalRun(t, "run-rehearsal", base))

		simulated, err := s.HasSimulation(context.Background(), "triage")
		if err != nil || simulated {
			t.Errorf("HasSimulation = %v err=%v, want false: one rehearsal is not a simulation of the agent",
				simulated, err)
		}
	})
	// The anchor: a battery beside it still counts, so the no above is the exclusion.
	run(t, "a battery beside it still counts", func(t *testing.T, s Store) {
		mustAppend(t, s, rehearsalRun(t, "run-rehearsal", base))
		mustAppend(t, s, batteryRun(t, "run-battery", base.Add(time.Hour)))

		simulated, err := s.HasSimulation(context.Background(), "triage")
		if err != nil || !simulated {
			t.Errorf("HasSimulation = %v err=%v, want true", simulated, err)
		}
	})
}
