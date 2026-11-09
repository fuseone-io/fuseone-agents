package trigger

import (
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// ErrNotAnExperiment refuses a configuration, or an experiment id, on a run that is not
// a simulated experiment run.
var ErrNotAnExperiment = errors.New("trigger: only a simulated experiment run carries a configuration")

/*
experimentOf is the configuration a run records, after the fence (NT-015, rule 2).

An experiment is a simulation, so its id needs a simulation beside it: the dry tool
layer is chosen by the run being simulated, and nothing else keeps a configured run away
from real tools. A configuration needs an experiment, so a battery carrying one is
refused as well as a real run: a battery is the evidence the start gate reads, and it has
to be the version as published.

An empty configuration is returned as none, so "the version as published" is recorded
one way only and the worker never has two to tell apart.
*/
func experimentOf(req Request) (*domain.ExperimentConfig, error) {
	if req.Experiment != "" && req.Simulation == "" {
		return nil, fmt.Errorf("%w: experiment %s is not simulated", ErrNotAnExperiment, req.Experiment)
	}
	if req.ExperimentConfig == nil || req.ExperimentConfig.IsZero() {
		return nil, nil
	}
	if req.Experiment == "" {
		return nil, ErrNotAnExperiment
	}
	config := *req.ExperimentConfig
	return &config, nil
}
