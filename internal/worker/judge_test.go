package worker

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A judge run is never planned as the agent it judges (NT-016 § Rollout).

Its input is the case and its version is the judged one, so a pool that resolved it as an
ordinary experiment run would run the judged agent itself on the case and spend money on
an answer nobody asked for. Until this worker can run a judge, it parks one, and it parks
one outside an experiment whatever it can do: a judge is honoured only on a simulated run
that names the experiment it judges.
*/

func judgeOpening() *domain.JudgeOpening {
	return &domain.JudgeOpening{
		Run: "run-exp-1", RubricDigest: "sha256:rubric", Provider: "anthropic", Model: "judge",
		CeilingMicros: 1000, Repetitions: 1, Repetition: 1,
	}
}

func TestTurn_aJudgeRun_isNeverPlannedAsTheJudgedAgent(t *testing.T) {
	t.Parallel()
	config := &domain.ExperimentConfig{Model: "other-model"}
	cases := []struct {
		name    string
		started domain.RunStartedPayload
		real    bool
	}{
		{"a judge run of an experiment", domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "jdg-1", Case: "case-1",
			Experiment: "exp-1", Judge: judgeOpening(),
		}, false},
		// The value the bug takes: a configuration beside the judge would otherwise send the
		// run down the experiment's path and plan the judged agent on another model.
		{"a judge run carrying an experiment's configuration", domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "jdg-1", Case: "case-1",
			Experiment: "exp-1", ExperimentConfig: config, Judge: judgeOpening(),
		}, false},
		{"a judge naming no experiment", domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "jdg-1", Case: "case-1",
			Judge: judgeOpening(),
		}, false},
		{"a judge on a real run", domain.RunStartedPayload{
			Trigger: "manual", Experiment: "exp-1", Judge: judgeOpening(),
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			openSimulated(t, store, c.started)
			planner := &flakyPlanner{}
			specs := &experimentSpecs{planner: planner}

			if c.real {
				turnOn(t, store, store, specs)
			} else {
				simulationTurn(t, store, specs)
			}

			if specs.declared != 0 || len(specs.asked) != 0 || planner.calls != 0 {
				t.Errorf("resolved the version %d times, the experiment %+v, planned %d times; want none",
					specs.declared, specs.asked, planner.calls)
			}
			if got := parkReason(t, store); got != "spec_unresolved" {
				t.Errorf("reason = %q, want the run parked as unresolved", got)
			}
		})
	}
}

// Which refusal matters once this worker runs judges: one outside an experiment stays
// refused, and only that one.
func TestJudgeRefusal_saysWhetherTheJudgeWasOutsideAnExperiment(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		started domain.RunStartedPayload
		want    error
	}{
		{"of an experiment", domain.RunStartedPayload{Simulated: true, Experiment: "exp-1", Judge: judgeOpening()}, errJudgeNotRunHere},
		{"naming no experiment", domain.RunStartedPayload{Simulated: true, Judge: judgeOpening()}, errJudgeOutsideExperiment},
		{"on a real run", domain.RunStartedPayload{Experiment: "exp-1", Judge: judgeOpening()}, errJudgeOutsideExperiment},
	} {
		if err := judgeRefusal("run-1", c.started); !errors.Is(err, c.want) {
			t.Errorf("%s: refusal = %v, want %v", c.name, err, c.want)
		}
	}
}
