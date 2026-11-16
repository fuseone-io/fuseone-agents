package model

import (
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
The registry's lookups, each asked at the scope a run will run in (NT-019,
phase 3). A provider is found only when its scope contains that one; the
installation's contains every scope. A refusal names the provider asked for and
never what else is configured, which would name other areas' providers.
*/

// Claim is a name the administration area configures, and where: claimed
// whether or not this process could build it, so a name switched off or
// sealed still holds its place, and its scope.
type Claim struct {
	Name  string
	Scope domain.Scope
}

// Configured reports whether name is a provider configured for a run at
// scope: registered here, or claimed by the administration area even when
// this process could not build it.
func (r *Registry) Configured(name string, at domain.Scope) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, held := r.providers[name]; held {
		return p.Scope.Contains(at)
	}
	claimed, ok := r.configured[name]
	return ok && claimed.Contains(at)
}

// Usable reports whether name is a provider this process built and would ask
// for a run at scope: switched on, its credential opened, its address whole.
// Configured also answers for a name that is claimed and none of those, which
// is right for a fallback a worker may still reach and wrong for an
// experiment, whose runs on such a name could only park.
func (r *Registry) Usable(name string, at domain.Scope) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, held := r.providers[name]
	return held && p.Scope.Contains(at)
}

// Planner builds the planner a run at scope plans on.
func (r *Registry) Planner(providerName string, at domain.Scope, cfg Config, tools ToolSchemas) (engine.Planner, error) {
	r.mu.RLock()
	p, ok := r.providers[providerName]
	r.mu.RUnlock()

	if !ok || !p.Scope.Contains(at) {
		return nil, fmt.Errorf("model: provider %q is not configured for this run's scope", providerName)
	}
	return r.plannerFrom(p, withAutoToolChoice(p, withPrice(p, cfg)), tools)
}

// FromEnvironment reports whether name is a provider the environment supplies,
// whether or not a console override replaces it now. It is the installation's,
// so no company or area may take the name (NT-019).
//
// Remembered apart from what is live, so a console override of the name, and
// its removal, do not make it forget the name is the environment's.
func (r *Registry) FromEnvironment(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, supplied := r.environment[name]
	return supplied
}

// restoreOrDrop puts the environment's provider back under a name the console
// no longer configures, or drops the name. Called with the lock held.
func (r *Registry) restoreOrDrop(name string) {
	if p, supplied := r.environment[name]; supplied {
		r.providers[name] = p
		return
	}
	delete(r.providers, name)
}
