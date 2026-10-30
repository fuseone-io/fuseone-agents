package spec

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/worker"
)

func declaredPair(s Spec) domain.ModelPair {
	return domain.ModelPair{Provider: s.Provider, Model: s.Model}
}

/*
fallback is the version's declared fallback, ready to plan with.

Built with everything else the definition says — effort, cache window, reply
language, instructions — under the fallback's provider and model, because the
author reviewed one agent with two possible pairs, not two agents.

A fallback this worker cannot build does not fail the resolution. The run asks
its declared pair until the ledger says otherwise, and most runs never switch:
an outage of the configuration for the fallback must not stop a run that never
needed it. It fails at the moment it is asked, which parks the run with the
reason on it.
*/
func (r *Resolver) fallback(s Spec) *worker.Fallback {
	if s.Fallback.IsZero() {
		return nil
	}
	planner, err := r.planner(s, s.Fallback)
	if err != nil {
		planner = unbuildable{err: err}
	}
	return &worker.Fallback{From: declaredPair(s), To: s.Fallback, Planner: planner}
}

// unbuildable is a fallback planner this worker could not construct.
type unbuildable struct{ err error }

func (u unbuildable) Plan(context.Context, engine.PlanInput) (engine.Proposal, error) {
	return engine.Proposal{}, u.err
}
