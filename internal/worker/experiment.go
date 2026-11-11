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

A judge run is never resolved as the version it judges (NT-016), in either pool: it parks,
whatever else its opening carries. See judgeRefusal.
*/
func (w *Worker) resolve(ctx context.Context, claim domain.Claim) (Resolution, error) {
	// Every pool reads the opening, the real one included: the judge mark is checked
	// before any resolver, so a judge run that reached the real pool never plans with
	// real tools (NT-016).
	started, err := w.openingOf(ctx, claim.RunID)
	if err != nil {
		return Resolution{}, err
	}
	if started.Judge != nil {
		return Resolution{}, judgeRefusal(claim.RunID, started)
	}
	if w.experiments == nil {
		return w.specs.Resolve(ctx, claim.AgentID, claim.VersionID)
	}
	config := experimentConfigOf(started)
	if config == nil {
		return w.specs.Resolve(ctx, claim.AgentID, claim.VersionID)
	}
	return w.experiments.ResolveExperiment(ctx, claim.AgentID, claim.VersionID, *config)
}

// openingOf is what a run recorded when it opened.
func (w *Worker) openingOf(ctx context.Context, run domain.RunID) (domain.RunStartedPayload, error) {
	steps, err := w.deps.Ledger.Read(ctx, run, domain.FirstSeq)
	if err != nil {
		return domain.RunStartedPayload{}, fmt.Errorf("read the opening of %s: %w", run, err)
	}
	if len(steps) == 0 || steps[0].Kind != domain.StepRunStarted {
		return domain.RunStartedPayload{}, fmt.Errorf("%s has no opening step", run)
	}
	// Read as the engine's fold reads it: no payload is an empty opening, so the fence
	// never parks a run the engine would advance.
	var started domain.RunStartedPayload
	if len(steps[0].Payload) == 0 {
		return started, nil
	}
	if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
		return domain.RunStartedPayload{}, fmt.Errorf("decode the opening of %s: %w", run, err)
	}
	return started, nil
}

// experimentConfigOf is the configuration an experiment run recorded, or none.
func experimentConfigOf(started domain.RunStartedPayload) *domain.ExperimentConfig {
	if !started.Simulated || started.Experiment == "" ||
		started.ExperimentConfig == nil || started.ExperimentConfig.IsZero() {
		return nil
	}
	return started.ExperimentConfig
}
