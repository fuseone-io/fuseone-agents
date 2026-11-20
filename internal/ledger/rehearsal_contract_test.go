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

/*
A rehearsal waiting for its author is not claimable (NT-018 phase 4): no worker holds it
while the author thinks. The author's answer makes it claimable again, so the next turn
records the call's result, and the author's decision on a held action does what a person's
does. The same rule in both stores, from the same projection.
*/
func TestSimulationClaim_aRehearsalWaitingForTheWorld_isClaimableOnlyOnceAnswered(t *testing.T) {
	at := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	step := func(seq int64, kind domain.StepKind, payload any) domain.Step {
		return domain.Step{RunID: "run-rehearsal", Seq: seq, Kind: kind, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v3", At: at.Add(time.Duration(seq) * time.Second), Payload: mustJSON(t, payload)}
	}
	run(t, "waiting, then answered", func(t *testing.T, s Store) {
		ctx := context.Background()
		mustAppend(t, s, rehearsalRun(t, "run-rehearsal", at))
		mustAppend(t, s, step(2, domain.StepToolCalled, domain.ToolCalledPayload{Tool: "crm.lookup", Effect: domain.EffectRead}))
		mustAppend(t, s, step(3, domain.StepWorldAwaited, domain.WorldAwaitedPayload{Tool: "crm.lookup", CallSeq: 2}))

		if claimed, err := s.Simulations().Claim(ctx, "sim-worker-1", time.Minute); err == nil {
			t.Fatalf("claimed %s while it waits for its author", claimed.RunID)
		}
		mustAppend(t, s, step(4, domain.StepWorldSupplied, domain.WorldSuppliedPayload{AtSeq: 3, Ref: "r", Digest: "d", By: "usr_ana"}))
		claimed, err := s.Simulations().Claim(ctx, "sim-worker-1", time.Minute)
		if err != nil || claimed.RunID != "run-rehearsal" {
			t.Fatalf("Claim = %+v (%v), want the answered rehearsal", claimed, err)
		}
	})
}

func TestSimulationClaim_aRehearsalDecided_isClaimableAgain(t *testing.T) {
	at := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	step := func(seq int64, kind domain.StepKind, payload any) domain.Step {
		return domain.Step{RunID: "run-decided", Seq: seq, Kind: kind, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v3", At: at.Add(time.Duration(seq) * time.Second), Payload: mustJSON(t, payload)}
	}
	run(t, "held, then decided", func(t *testing.T, s Store) {
		ctx := context.Background()
		mustAppend(t, s, rehearsalRun(t, "run-decided", at))
		mustAppend(t, s, step(2, domain.StepApprovalRequested, domain.ApprovalRequestedPayload{Tool: "crm.refund"}))
		if claimed, err := s.Simulations().Claim(ctx, "sim-worker-1", time.Minute); err == nil {
			t.Fatalf("claimed %s while the Gate holds it", claimed.RunID)
		}
		mustAppend(t, s, step(3, domain.StepRehearsalDecided, domain.RehearsalDecidedPayload{AtSeq: 2, Approved: true, By: "usr_ana"}))
		if claimed, err := s.Simulations().Claim(ctx, "sim-worker-1", time.Minute); err != nil || claimed.RunID != "run-decided" {
			t.Fatalf("Claim = %+v (%v), want the decided rehearsal", claimed, err)
		}
	})
}
