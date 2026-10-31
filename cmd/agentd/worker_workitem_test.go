package main

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/workitem"
)

// The claim has to be describable where the planner looks and classified
// where the engine asks, from the worker's real wiring. A layer that exists
// but was never stacked here plans every role without the claim, silently:
// the planner skips a tool it cannot describe (#206).
func TestWorkerDeps_describeAndClassifyTheWorkItemClaim(t *testing.T) {
	t.Parallel()

	content := engine.NewMemoryContent()
	p := &workerParts{content: content, catalog: tools.NewCatalog(content)}
	deps := p.deps(gate.New(), nil)

	if _, _, _, ok := p.toolSchemas().Schema(domain.ToolWorkItemClaim); !ok {
		t.Error("the planner's schema view cannot describe the claim")
	}
	if _, _, _, ok := deps.Schemas.Schema(domain.ToolWorkItemClaim); !ok {
		t.Error("the engine's schema view cannot describe the claim")
	}
	if effect, ok := deps.Catalog.Effect(domain.ToolWorkItemClaim); !ok || effect != domain.EffectWrite {
		t.Errorf("Catalog.Effect(claim) = %s, %v; want write", effect, ok)
	}
	_, err := deps.Tools.Invoke(context.Background(), engine.Call{Tool: domain.ToolWorkItemClaim})
	if !errors.Is(err, workitem.ErrEngineExecuted) {
		t.Errorf("Tools.Invoke(claim) = %v; the outermost layer must refuse to forward it", err)
	}
}
