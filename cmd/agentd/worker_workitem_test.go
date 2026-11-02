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
func TestWorkerDeps_describeAndClassifyTheWorkItemClaimAndLink(t *testing.T) {
	t.Parallel()

	content := engine.NewMemoryContent()
	p := &workerParts{content: content, catalog: tools.NewCatalog(content)}
	deps := p.deps(gate.New(), nil)

	for _, tool := range []domain.ToolID{domain.ToolWorkItemClaim, domain.ToolWorkItemLink} {
		if _, _, _, ok := p.toolSchemas().Schema(tool); !ok {
			t.Errorf("the planner's schema view cannot describe %s", tool)
		}
		if _, _, _, ok := deps.Schemas.Schema(tool); !ok {
			t.Errorf("the engine's schema view cannot describe %s", tool)
		}
		if effect, ok := deps.Catalog.Effect(tool); !ok || effect != domain.EffectWrite {
			t.Errorf("Catalog.Effect(%s) = %s, %v; want write", tool, effect, ok)
		}
		_, err := deps.Tools.Invoke(context.Background(), engine.Call{Tool: tool})
		if !errors.Is(err, workitem.ErrEngineExecuted) {
			t.Errorf("Tools.Invoke(%s) = %v; the outermost layer must refuse to forward it", tool, err)
		}
	}
}
