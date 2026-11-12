package engine

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

// scopedCatalog is the harness's catalogue with one tool moved to another scope.
type scopedCatalog struct {
	staticCatalog
	tool  domain.ToolID
	scope domain.Scope
}

func (c scopedCatalog) Scope(id domain.ToolID) (domain.Scope, bool) {
	if id == c.tool {
		return c.scope, true
	}
	return c.staticCatalog.Scope(id)
}

/*
A tool of another area is stopped by the Gate, and the ledger says so (NT-019).

The pack names the tool, so nothing before the Gate objects. What stops it is
where the tool belongs, and that is recorded with the decision: a replay, or
anybody reading the trail, can see why without asking the catalogue again.
*/
func TestAdvance_aToolOfASiblingArea_isBlockedByScopeAndNeverCalled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	finance := domain.Scope{Company: "acme", Area: "finance"}

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	h.runner.deps.Catalog = scopedCatalog{
		staticCatalog: h.runner.deps.Catalog.(staticCatalog), tool: "crm.lookup", scope: finance,
	}

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var decided domain.GateDecidedPayload
	if err := h.payloadOf(t, domain.StepGateDecided, &decided); err != nil {
		t.Fatalf("read the decision: %v", err)
	}
	if decided.Verdict != domain.VerdictBlock || decided.Rule != gate.RuleScope {
		t.Errorf("decision = %v by %q, want block by %q", decided.Verdict, decided.Rule, gate.RuleScope)
	}
	if decided.ToolScope == nil || decided.ToolScope.Scope() != finance {
		t.Errorf("recorded tool scope = %v, want %s", decided.ToolScope, finance)
	}
	if len(h.tools.invocations) != 0 {
		t.Errorf("the tool was called %d times", len(h.tools.invocations))
	}
}

// A tool of the run's own scope, or above it, runs as it always did.
func TestAdvance_aToolOfTheInstallation_recordsItsScopeAndRuns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var decided domain.GateDecidedPayload
	if err := h.payloadOf(t, domain.StepGateDecided, &decided); err != nil {
		t.Fatalf("read the decision: %v", err)
	}
	installation := domain.Scope{Company: domain.Installation}
	if decided.ToolScope == nil || decided.ToolScope.Scope() != installation {
		t.Errorf("recorded tool scope = %v, want %s", decided.ToolScope, installation)
	}
	if len(h.tools.invocations) != 1 {
		t.Errorf("the tool was called %d times, want once", len(h.tools.invocations))
	}
	if decided.Builtin != gate.BuiltinVersion() {
		t.Errorf("recorded builtin = %q, want %q", decided.Builtin, gate.BuiltinVersion())
	}
}
