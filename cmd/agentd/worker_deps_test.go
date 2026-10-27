package main

import (
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/tools"
)

// The engine's unavailable-tools check is opt-in by wiring, so the wiring is
// where it dies silently: a worker whose deps carry no schema view plans
// toolless runs exactly as before #148, and every engine-level test keeps
// passing.
func TestWorkerDeps_carryTheSchemaViewAndTheConfiguredServers(t *testing.T) {
	t.Parallel()

	content := engine.NewMemoryContent()
	p := &workerParts{content: content, catalog: tools.NewCatalog(content)}

	bare := p.deps(gate.New(), nil)
	if bare.Schemas == nil {
		t.Error("deps carry no schema view; the engine cannot see a missing pack tool")
	}
	if bare.Servers != nil {
		t.Error("without an administration store, no server can be expected")
	}

	p.integrations = &admin.Integrations{}
	wired := p.deps(gate.New(), nil)
	if wired.Servers == nil {
		t.Error("deps carry no configured-server view; an outage reads as a removal")
	}
}
