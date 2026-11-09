package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// ExperimentSpecs resolves a version under an experiment's configuration, declared here
// by the consumer.
type ExperimentSpecs interface {
	ResolveExperiment(
		ctx context.Context, agent domain.AgentID, version domain.VersionID, config domain.ExperimentConfig,
	) (Resolution, error)
}

// WithExperiments wires how the simulation pool resolves an experiment run.
func (w *Worker) WithExperiments(e ExperimentSpecs) *Worker {
	w.experiments = e
	return w
}

/*
resolve is what a claimed run plans with.

A pool with experiments wired, which is the simulation pool, reads the run's opening step
and honours a configuration only on a simulated run that names an experiment (NT-015,
rule 2). The opener refuses any other already; this is the second door, so a run that
reached the pool with a configuration some other way plans as the version published it.
An experiment with no configuration is the version as published too, fallback and all.
*/
func (w *Worker) resolve(ctx context.Context, claim domain.Claim) (Resolution, error) {
	if w.experiments == nil {
		return w.specs.Resolve(ctx, claim.AgentID, claim.VersionID)
	}
	config, err := w.experimentConfig(ctx, claim.RunID)
	if err != nil {
		return Resolution{}, err
	}
	if config == nil {
		return w.specs.Resolve(ctx, claim.AgentID, claim.VersionID)
	}
	return w.experiments.ResolveExperiment(ctx, claim.AgentID, claim.VersionID, *config)
}

// experimentConfig is the configuration an experiment run recorded, or none.
func (w *Worker) experimentConfig(ctx context.Context, run domain.RunID) (*domain.ExperimentConfig, error) {
	steps, err := w.deps.Ledger.Read(ctx, run, domain.FirstSeq)
	if err != nil {
		return nil, fmt.Errorf("read the opening of %s: %w", run, err)
	}
	if len(steps) == 0 || steps[0].Kind != domain.StepRunStarted {
		return nil, fmt.Errorf("%s has no opening step", run)
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
		return nil, fmt.Errorf("decode the opening of %s: %w", run, err)
	}
	if !started.Simulated || started.Experiment == "" ||
		started.ExperimentConfig == nil || started.ExperimentConfig.IsZero() {
		return nil, nil
	}
	return started.ExperimentConfig, nil
}
