package trigger_test

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/trigger"
)

/*
A configuration reaches experiment runs only (NT-015 § Experiments, rule 2).

The dry tool layer is what keeps a simulated run from reaching a real tool, and it is
chosen by the run being simulated. A configuration honoured on any other run would let a
real run plan on a model nobody published, and one honoured on a battery would make the
evidence the start gate reads about a version describe a model it never runs on. So the
opener refuses both, with the moves an adversary would make: a configuration on a request
that is not simulated, and one on a battery.
*/
func TestOpen_configurationOutsideAnExperiment_isRefusedAndOpensNothing(t *testing.T) {
	t.Parallel()
	another := &domain.ExperimentConfig{Provider: "openai", Model: "gpt-5.5"}
	cases := []struct {
		name string
		req  trigger.Request
	}{
		{"a configuration on a request that is not simulated",
			trigger.Request{ExperimentConfig: another}},
		{"a configuration on a battery",
			trigger.Request{Simulation: "sim-1", Case: "case-1", ExperimentConfig: another}},
		{"an experiment on a request that is not simulated",
			trigger.Request{Experiment: "exp-1", ExperimentConfig: another}},
		{"an experiment with no configuration, not simulated",
			trigger.Request{Experiment: "exp-1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			opener, store := openerFor(t)
			req := c.req
			req.Agent, req.IdemKey, req.Trigger = "triage", "intent-1", "simulation"

			_, err := opener.Open(t.Context(), req)
			if !errors.Is(err, trigger.ErrNotAnExperiment) {
				t.Fatalf("Open = %v, want ErrNotAnExperiment", err)
			}
			if _, err := store.RunByIdemKey(t.Context(), "intent-1"); err == nil {
				t.Error("a refused request still opened a run")
			}
		})
	}
}

// An experiment opens as a simulated run that records its id and what it overrode, and a
// configuration that overrides nothing is recorded as no configuration at all: one way to
// say "the version as published", so the worker cannot read two.
func TestOpen_experiment_recordsItsIdAndConfiguration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		config *domain.ExperimentConfig
		want   *domain.ExperimentConfig
	}{
		{"no configuration", nil, nil},
		{"an empty configuration", &domain.ExperimentConfig{}, nil},
		{"only effort", &domain.ExperimentConfig{Effort: "low"}, &domain.ExperimentConfig{Effort: "low"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			opener, store := openerFor(t)

			got, err := opener.Open(t.Context(), trigger.Request{
				Agent: "triage", IdemKey: "intent-1", Trigger: "simulation",
				Simulation: "exp-1", Case: "case-1", Experiment: "exp-1", ExperimentConfig: c.config,
			})
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			steps, err := store.Read(t.Context(), got.RunID, domain.FirstSeq)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			var started domain.RunStartedPayload
			decode(t, steps[0].Payload, &started)
			if !started.Simulated || started.Experiment != "exp-1" {
				t.Fatalf("started = %+v, want a simulated run of exp-1", started)
			}
			if (started.ExperimentConfig == nil) != (c.want == nil) ||
				(c.want != nil && *started.ExperimentConfig != *c.want) {
				t.Errorf("configuration = %+v, want %+v", started.ExperimentConfig, c.want)
			}
		})
	}
}
