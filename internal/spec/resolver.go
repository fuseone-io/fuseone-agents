package spec

import (
	"context"
	"fmt"
	"sync"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/worker"
)

// Resolver turns an agent version into something a worker can run.
//
// It is the seam between the three registries an installation configures —
// agent definitions, model providers, and the tool catalogue — and the loop,
// which knows about none of them.
type Resolver struct {
	specs     *Store
	providers *model.Registry
	tools     model.ToolSchemas

	// planners are cached per agent version. A planner owns an HTTP client and
	// a stable system prompt, and the version is a content digest, so a cached
	// planner can never be stale: different bytes are a different version.
	mu       sync.RWMutex
	planners map[domain.VersionID]engine.Planner
}

func NewResolver(specs *Store, providers *model.Registry, tools model.ToolSchemas) *Resolver {
	return &Resolver{
		specs:     specs,
		providers: providers,
		tools:     tools,
		planners:  make(map[domain.VersionID]engine.Planner),
	}
}

var _ worker.Specs = (*Resolver)(nil)

// Resolve returns the run configuration and planner for one agent version.
//
// An empty version resolves to the current one, which is what a fresh trigger
// gets. A run already in flight passes the version it was pinned to, so
// publishing never changes what it is doing mid-run (PRD DE-09).
func (r *Resolver) Resolve(_ context.Context, agent domain.AgentID, version domain.VersionID) (worker.Resolution, error) {
	spec, err := r.specs.Get(agent, version)
	if err != nil {
		return worker.Resolution{}, err
	}

	planner, err := r.planner(spec)
	if err != nil {
		return worker.Resolution{}, err
	}

	return worker.Resolution{
		Start: engine.Start{
			// The pack is frozen here, at the start of the run, and only ever
			// shrinks from this point (PRD SE-04).
			Pack:    gate.NewPack(spec.Tools...),
			Budget:  spec.Budget,
			Trigger: "worker",
		},
		Planner: planner,
	}, nil
}

func (r *Resolver) planner(spec Spec) (engine.Planner, error) {
	r.mu.RLock()
	cached, ok := r.planners[spec.Version]
	r.mu.RUnlock()
	if ok {
		return cached, nil
	}

	planner, err := r.providers.Planner(spec.Provider, model.Config{
		Model:  spec.Model,
		Effort: spec.Effort,
		// The body of the definition is the system prompt: what the author
		// reviewed is what the model receives.
		SystemPrompt: spec.Instructions,
	}, r.tools)
	if err != nil {
		return nil, fmt.Errorf("spec: %s@%s: %w", spec.ID, spec.Version, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.planners[spec.Version] = planner
	return planner, nil
}
