package ledger_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
A judge run beside the experiment it judges (NT-016).

It names the experiment, so the doors that keep experiments out of batteries, the start
gate and FU-10 keep it out with no change. It is not one of the experiment's runs, so the
readers that group an experiment's runs leave it out, on Postgres and on the fake alike.
*/

// judgeRun is a judge run of an experiment's run, opened at a moment. caseID is the judged
// run's case, or empty: either way it is not a case of the experiment.
func judgeRun(
	t *testing.T, run domain.RunID, experiment, caseID string, at time.Time,
) domain.Step {
	t.Helper()
	st := startedAt(run, at)
	st.AgentID, st.IdemKey = "triage", string(run)
	st.Payload = mustJSON(t, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "jdg-1",
		Case: caseID, DatasetVersion: 2, Experiment: experiment,
		Judge: &domain.JudgeOpening{
			Run: "run-1", RubricDigest: "sha256:rubric", Provider: "anthropic", Model: "judge",
			CeilingMicros: 1000, Repetitions: 1, Repetition: 1,
		},
	})
	return st
}

func TestExperiments_judgeRunsNamingTheExperiment_areNotItsRuns(t *testing.T) {
	base := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	run(t, "judge runs beside an experiment", func(t *testing.T, s Store) {
		ctx := context.Background()
		mustAppend(t, s, openedExperiment(t, "run-1", "triage", "exp-1", base))
		mustAppend(t, s, openedExperiment(t, "run-2", "triage", "exp-1", base.Add(time.Minute)))
		// Earlier than every run of the experiment, so a reader that let it in would take
		// its opening as the experiment's, and its empty case as one more case.
		mustAppend(t, s, judgeRun(t, "run-judge-early", "exp-1", "", base.Add(-time.Hour)))
		mustAppend(t, s, judgeRun(t, "run-judge", "exp-1", "case-9", base.Add(time.Hour)))

		got, err := s.Experiments(ctx, "triage", 10)
		if err != nil || len(got) != 1 {
			t.Fatalf("Experiments = %+v err=%v, want exp-1 alone", got, err)
		}
		e := got[0]
		if e.ID != "exp-1" || e.Runs != 2 || e.Cases != 1 ||
			e.Opening.RunID != "run-1" || !e.OpenedAt.Equal(base) {
			t.Errorf("listing = %s runs %d cases %d first %s at %v; want exp-1, 2 runs, 1 case, run-1 at %v",
				e.ID, e.Runs, e.Cases, e.Opening.RunID, e.OpenedAt, base)
		}
	})
}

// Judge runs alone open no experiment: a listing made only of them would be a judgment
// offered to the console as something to compare.
func TestExperiments_onlyJudgeRuns_listNothing(t *testing.T) {
	base := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	run(t, "only judge runs", func(t *testing.T, s Store) {
		mustAppend(t, s, judgeRun(t, "run-judge", "exp-1", "case-1", base))

		got, err := s.Experiments(context.Background(), "triage", 10)
		if err != nil || len(got) != 0 {
			t.Errorf("Experiments = %+v err=%v, want none", got, err)
		}
	})
}

// The doors already deployed hold for a judge run: it names an experiment, and every one
// of them leaves a run that does out (NT-015). Asserted here so a later change to how a
// judge run is opened cannot quietly walk it through one.
func TestGateDoors_judgeRun_isNeitherABatteryNorASimulation(t *testing.T) {
	base := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	run(t, "a judge run alone", func(t *testing.T, s Store) {
		ctx := context.Background()
		judge := judgeRun(t, "run-judge", "exp-1", "case-1", base)
		judge.VersionID = "v3"
		mustAppend(t, s, judge)

		if has, err := s.HasSimulation(ctx, "triage"); err != nil || has {
			t.Errorf("HasSimulation = %v err=%v, want false", has, err)
		}
		if got, err := s.Batteries(ctx, "triage", "v3", 10); err != nil || len(got) != 0 {
			t.Errorf("Batteries = %v err=%v, want none", got, err)
		}
		if at, ran, err := s.LastBatteryAt(ctx, "triage", "v3"); err != nil || ran {
			t.Errorf("LastBatteryAt = %v ran=%v err=%v, want no battery", at, ran, err)
		}
	})
}
