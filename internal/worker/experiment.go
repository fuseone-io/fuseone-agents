package worker

import (
	"context"
	"encoding/json"
	"errors"
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
		return w.resolveJudge(ctx, claim.RunID, started)
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

// Openings reads a run's first step alone, declared here by the consumer. Every turn of
// every pool asks for it, so it must cost one step however long the run has grown.
type Openings interface {
	Opening(ctx context.Context, runID domain.RunID) (domain.Step, error)
}

// errNoOpenings is a ledger that cannot read an opening alone. Failing loudly beats
// reading the whole run on every turn, or skipping the judge fence.
var errNoOpenings = errors.New("the ledger cannot read a run's opening alone")

// openingOf is what a run recorded when it opened.
func (w *Worker) openingOf(ctx context.Context, run domain.RunID) (domain.RunStartedPayload, error) {
	openings, ok := w.deps.Ledger.(Openings)
	if !ok {
		return domain.RunStartedPayload{}, errNoOpenings
	}
	first, err := openings.Opening(ctx, run)
	if err != nil {
		return domain.RunStartedPayload{}, fmt.Errorf("read the opening of %s: %w", run, err)
	}
	if first.Kind != domain.StepRunStarted {
		return domain.RunStartedPayload{}, fmt.Errorf("%s has no opening step", run)
	}
	// Read as the engine's fold reads it: no payload is an empty opening, so the fence
	// never parks a run the engine would advance.
	var started domain.RunStartedPayload
	if len(first.Payload) == 0 {
		return started, nil
	}
	if err := json.Unmarshal(first.Payload, &started); err != nil {
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
