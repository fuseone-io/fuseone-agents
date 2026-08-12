package simulate_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
)

// advance drives one run the way the simulation pool does: the same runner,
// one turn at a time, with dependencies from the one constructor that installs
// the dry tool layer.
func advance(t *testing.T, deps engine.Deps, planner engine.Planner, id domain.RunID) {
	t.Helper()

	runner := engine.NewRunner(simulate.Deps(withPlanner(deps, planner)))
	start := engine.Start{
		RunID: id, Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "suporte", VersionID: "v1", Trigger: "simulation",
		Pack: gate.NewPack("crm.lookup"),
		// The step ceiling is the turn bound. A run limited only by money
		// would spend the whole ceiling of every case in a set of fifty
		// before anybody saw a report.
		Budget: domain.Budget{Micros: 1_000_000, Steps: 20},
	}
	for range 20 {
		status, err := runner.Advance(t.Context(), start)
		if err != nil {
			t.Fatalf("advance %s: %v", id, err)
		}
		switch status.Phase {
		case engine.PhaseFinished, engine.PhaseParked, engine.PhaseAwaitingApproval:
			return
		}
	}
	t.Fatalf("run %s never settled", id)
}

func withPlanner(deps engine.Deps, planner engine.Planner) engine.Deps {
	deps.Planner = planner
	return deps
}

func TestGather_foldsEveryRunTheSimulationOpened(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	live := &liveTools{}
	deps := depsFor(store, content, live)

	opened, err := simulate.Open(t.Context(), openerFor(store, content), batchOf(`{"n":1}`, `{"n":2}`))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, id := range opened.Runs {
		advance(t, deps, onceThenDone{tool: "crm.lookup"}, id)
	}

	got, err := simulate.Gather(t.Context(), store, "sim-1")
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	// The property the whole feature rests on: a run marked simulated has no
	// path to a real system, whatever the pool was built from.
	if live.calls != 0 {
		t.Fatalf("the real tool layer was invoked %d times", live.calls)
	}
	if len(got.Cases) != 2 || got.Running {
		t.Fatalf("report = %+v", got)
	}
	if got.Agent != "suporte" || got.Version != "v1" {
		t.Errorf("report names %s@%s", got.Agent, got.Version)
	}
	for i, c := range got.Cases {
		if c.Settled != simulate.SettledFinished {
			t.Errorf("case %d settled %q", i+1, c.Settled)
		}
		// It got as far as the tool layer, so the absence above is the dry
		// layer standing in rather than the run never having proposed
		// anything.
		if len(c.Acted) != 1 || !c.Acted[0].Reached {
			t.Errorf("case %d acted %+v", i+1, c.Acted)
		}
	}
}

func TestGather_aCaseStillBeingAdvanced_reportsTheSimulationAsRunning(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	opened, err := simulate.Open(t.Context(), openerFor(store, content), batchOf(`{"n":1}`, `{"n":2}`))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Only the first is driven, as though the pool had not reached the second.
	advance(t, depsFor(store, content, &liveTools{}), onceThenDone{tool: "crm.lookup"}, opened.Runs[0])

	got, err := simulate.Gather(t.Context(), store, "sim-1")
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	// Derived from the runs rather than tracked beside them: the runs are the
	// queue, so a simulation is still going exactly when one of its runs is.
	if !got.Running {
		t.Errorf("report = %+v, want it still running", got)
	}
	if len(got.Cases) != 2 {
		t.Errorf("cases = %d, want both rows from the moment they opened", len(got.Cases))
	}
}

func TestGather_reportsWhereTheGateWouldHaveStopped(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	opened, err := simulate.Open(t.Context(), openerFor(store, content), batchOf(`{"n":1}`))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Outside the pack the version was frozen with: the Gate refuses it on
	// capability, and this is the row the author most needs to see.
	advance(t, depsFor(store, content, &liveTools{}), onceThenDone{tool: "crm.refund"}, opened.Runs[0])

	got, err := simulate.Gather(t.Context(), store, "sim-1")
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	act := got.Cases[0].Acted[0]
	if act.Verdict != domain.VerdictBlock || act.Rule != gate.RuleCapability {
		t.Errorf("act = %+v, want a capability block", act)
	}
	if act.Reached {
		t.Error("a blocked proposal is reported as having reached the tool layer")
	}
}

func TestGather_aSimulationNobodyRan_isEmptyRatherThanAnError(t *testing.T) {
	t.Parallel()

	got, err := simulate.Gather(t.Context(), ledger.NewMemory(), "sim-never")
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(got.Cases) != 0 || got.Running {
		t.Errorf("report = %+v", got)
	}
}

func TestGather_readsWhichCorpusCaseARunReplayed(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	batch := simulate.Batch{
		ID: "sim-1", Agent: "suporte", By: "ana",
		Cases: []simulate.Occurrence{{ID: "reg-7", Input: []byte(`{"n":1}`)}},
	}
	if _, err := simulate.Open(t.Context(), openerFor(store, content), batch); err != nil {
		t.Fatalf("Open: %v", err)
	}

	got, err := simulate.Gather(t.Context(), store, "sim-1")
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	// Without this the battery would match a correction to a run by position,
	// and the ledger's order is not the corpus's.
	if len(got.Cases) != 1 || got.Cases[0].ID != "reg-7" {
		t.Errorf("cases = %+v, want the corpus case named", got.Cases)
	}
}
