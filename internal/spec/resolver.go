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

/*
Definitions resolve a published version to what it says, declared here by the
consumer.

Both stores satisfy it, and which one a process uses is the difference between
the two ways this platform is installed. With a database the registry is the
source: publishing is an interface action rather than a deploy (PRD DE-07), so
an agent authored in the console has no file anywhere and resolving from disk
would leave it visible on every screen, triggerable, and unable to run.

Without one — a laptop, a test — the directory of definitions stands in.
*/
type Definitions interface {
	Get(ctx context.Context, agent domain.AgentID, version domain.VersionID) (Spec, error)
}

// Resolver turns an agent version into something a worker can run.
//
// It is the seam between the three registries an installation configures —
// agent definitions, model providers, and the tool catalogue — and the loop,
// which knows about none of them.
type Resolver struct {
	specs     Definitions
	providers *model.Registry
	tools     model.ToolSchemas

	// planners are cached per agent version and price revision. A planner owns
	// an HTTP client, a stable system prompt and the rate it accounts with:
	// the version is immutable, but the price table is live administration
	// state.
	mu       sync.RWMutex
	planners map[domain.VersionID]cachedPlanner
}

func NewResolver(specs Definitions, providers *model.Registry, tools model.ToolSchemas) *Resolver {
	return &Resolver{
		specs:     specs,
		providers: providers,
		tools:     tools,
		planners:  make(map[domain.VersionID]cachedPlanner),
	}
}

var _ worker.Specs = (*Resolver)(nil)

// Resolve returns the run configuration and planner for one agent version.
//
// An empty version resolves to the current one, which is what a fresh trigger
// gets. A run already in flight passes the version it was pinned to, so
// publishing never changes what it is doing mid-run (PRD DE-09).
func (r *Resolver) Resolve(ctx context.Context, agent domain.AgentID, version domain.VersionID) (worker.Resolution, error) {
	spec, err := r.specs.Get(ctx, agent, version)
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
			Steps:   envelopes(spec),
			Budget:  spec.Budget,
			Trigger: "worker",
		},
		Planner: planner,
	}, nil
}

func (r *Resolver) planner(spec Spec) (engine.Planner, error) {
	for {
		priceRevision := r.providers.PriceRevision()
		r.mu.RLock()
		cached, ok := r.planners[spec.Version]
		r.mu.RUnlock()
		if ok && cached.priceRevision == priceRevision {
			return cached.planner, nil
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
		if r.providers.PriceRevision() != priceRevision {
			continue
		}

		r.mu.Lock()
		r.planners[spec.Version] = cachedPlanner{
			planner:       planner,
			priceRevision: priceRevision,
		}
		r.mu.Unlock()
		return planner, nil
	}
}

type cachedPlanner struct {
	planner       engine.Planner
	priceRevision uint64
}

// envelopes hands the engine the step list as plain tool sets. The engine
// cannot import this package — dependencies point inward — so the shape
// crosses as data.
func envelopes(s Spec) []engine.Envelope {
	if len(s.Steps) == 0 {
		return nil
	}
	out := make([]engine.Envelope, 0, len(s.Steps))
	for _, step := range s.Steps {
		out = append(out, engine.Envelope{
			Name: step.Name, Reaches: step.Reaches,
			StopsWhen: step.StopsWhen,
			Model:     step.Model, Effort: step.Effort,
		})
	}
	return out
}
