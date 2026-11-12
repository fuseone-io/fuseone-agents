package main

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/memory"
	"github.com/fuseone/agents/internal/tools"
)

/*
The platform's own tools run from every scope (NT-019, phase 0).

The Gate stops a tool unless its scope contains the run's, and a tool nobody
declared has the zero scope, which contains nothing. So every one of the
platform's tools must be declared, by the layer that serves it, on the chain
the worker really builds. The set is walked from domain.PlatformTools, not
restated: a tool added to it later is covered here without editing this test.
*/
func TestWorkerChain_everyPlatformTool_isTheInstallationsAndTheGateAdmitsIt(t *testing.T) {
	t.Parallel()
	content := engine.NewMemoryContent()
	parts := &workerParts{
		content: content, memory: memory.NewMemory(), catalog: tools.NewCatalog(nil),
	}
	parts.native = connectortools.New(parts.catalog, parts.catalog, content, nil).
		WithTicketAnswers(connectortools.NewTicketAnswers(content, nil))
	deps := parts.deps(gate.New(), nil)
	area := domain.Scope{Company: "acme", Area: "cx"}

	for _, id := range domain.PlatformTools() {
		scope, ok := deps.Catalog.Scope(id)
		if !ok || scope != (domain.Scope{Company: domain.Installation}) {
			t.Errorf("%s: scope = %s (declared %v), want the installation's", id, scope, ok)
			continue
		}
		effect, _ := deps.Catalog.Effect(id)
		decision, err := gate.New().Evaluate(context.Background(), gate.Request{
			Scope: area, RunID: "run-1", AgentID: "triage", Seq: 2,
			Tool: id, Effect: effect, ToolScope: scope,
			Pack: gate.NewPack(id), Stage: domain.StageAutonomous,
			Budget: domain.Budget{Micros: 1_000_000, ToolCalls: 10},
		})
		if err != nil {
			t.Fatalf("%s: Evaluate: %v", id, err)
		}
		if decision.Rule == gate.RuleScope {
			t.Errorf("%s: stopped by scope from %s: %s", id, area, decision.Reason)
		}
	}
}

// An id no layer serves is nobody's, so the anchor above cannot pass because the
// scope check is missing.
func TestWorkerChain_anIdNoLayerServes_isStoppedByScope(t *testing.T) {
	t.Parallel()
	content := engine.NewMemoryContent()
	parts := &workerParts{
		content: content, memory: memory.NewMemory(), catalog: tools.NewCatalog(nil),
	}
	parts.native = connectortools.New(parts.catalog, parts.catalog, content, nil)
	deps := parts.deps(gate.New(), nil)

	const unclaimed domain.ToolID = "nobody.serves_this"
	scope, ok := deps.Catalog.Scope(unclaimed)
	if ok || scope != (domain.Scope{}) {
		t.Fatalf("scope = %s (declared %v), want none", scope, ok)
	}
	decision, err := gate.New().Evaluate(context.Background(), gate.Request{
		Scope: domain.Scope{Company: "acme", Area: "cx"}, RunID: "run-1", AgentID: "triage", Seq: 2,
		Tool: unclaimed, Effect: domain.EffectRead, ToolScope: scope,
		Pack: gate.NewPack(unclaimed), Stage: domain.StageAutonomous,
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Verdict != domain.VerdictBlock || decision.Rule != gate.RuleScope {
		t.Errorf("decision = %v by %q, want block by %q", decision.Verdict, decision.Rule, gate.RuleScope)
	}
}
