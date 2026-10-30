package spec

import "github.com/fuseone/agents/internal/domain"

/*
fallbackProblems reports a fallback pair that could not do its job.

Whether the provider is one this installation configures is not a question the
parser can answer — a file is read before any registry exists — so publishing
asks it, against the live provider list. What is checked here is what the
definition says about itself.
*/
func (s Spec) fallbackProblems() []string {
	if s.Fallback.IsZero() {
		return nil
	}
	if s.Fallback.Provider == "" || s.Fallback.Model == "" {
		return []string{"fallback needs a provider and a model"}
	}
	// The pair the run already asks: switching to it would be retrying under
	// another name, and the trail would record a switch that never happened.
	if s.Fallback == (domain.ModelPair{Provider: s.Provider, Model: s.Model}) {
		return []string{"fallback is the agent's own provider and model; name a different pair"}
	}
	return nil
}

func valueOf(p *domain.ModelPair) domain.ModelPair {
	if p == nil {
		return domain.ModelPair{}
	}
	return *p
}

func pairOrNil(p domain.ModelPair) *domain.ModelPair {
	if p.IsZero() {
		return nil
	}
	return &p
}
