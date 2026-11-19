package spec

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/worker"
)

/*
ResolveExperiment resolves a version under an experiment's configuration (NT-015).

An empty configuration is the version as published, declared fallback and all. Any other
drops the fallback, a partial one included: the experiment asked about one model, and a
fallback that switched to another would report that model while measuring something else.
When the chosen model's own retries run out the run parks, and the report says so.

Every field is optional and an absent one is the version's own, so a provider alone runs
the published model under it. The effective pair goes through the same registry a
published agent's does, so a provider this installation has not configured is refused
here rather than reached.
*/
func (r *Resolver) ResolveExperiment(
	ctx context.Context, claim domain.Claim, config domain.ExperimentConfig,
) (worker.Resolution, error) {
	if config.IsZero() {
		return r.Resolve(ctx, claim)
	}
	spec, err := r.specs.Get(ctx, claim.AgentID, claim.VersionID)
	if err != nil {
		return worker.Resolution{}, err
	}
	return r.resolveDefinition(spec, claim, config)
}

// resolveDefinition is what a worker runs one definition with: as declared,
// fallback and all, under an empty configuration; on the configuration's pair,
// with no fallback, under any other (NT-015).
func (r *Resolver) resolveDefinition(
	spec Spec, claim domain.Claim, config domain.ExperimentConfig,
) (worker.Resolution, error) {
	if config.IsZero() {
		planner, err := r.planner(spec, declaredPair(spec), claim.Scope)
		if err != nil {
			return worker.Resolution{}, err
		}
		return resolution(spec, planner, r.fallback(spec, claim.Scope)), nil
	}
	pair := declaredPair(spec)
	if config.Provider != "" {
		pair.Provider = config.Provider
	}
	if config.Model != "" {
		pair.Model = config.Model
	}
	if config.Effort != "" {
		spec.Effort = config.Effort
	}
	planner, err := r.planner(spec, pair, claim.Scope)
	if err != nil {
		return worker.Resolution{}, fmt.Errorf("experiment on %s/%s: %w", pair.Provider, pair.Model, err)
	}
	return resolution(spec, planner, nil), nil
}
