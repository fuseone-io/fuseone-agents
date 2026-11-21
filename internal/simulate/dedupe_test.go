package simulate_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/dedupe"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
)

// countingDedupe is the shared semantic-dedupe store, counting every use. A real run of the
// same agent and scope reads and reserves in it.
type countingDedupe struct{ uses int }

func (d *countingDedupe) Lookup(context.Context, dedupe.Key, time.Time) (dedupe.Record, bool, error) {
	d.uses++
	return dedupe.Record{}, false, nil
}

func (d *countingDedupe) Reserve(_ context.Context, _ dedupe.Key, run domain.RunID, _ time.Duration, _ time.Time) (dedupe.Record, error) {
	d.uses++
	return dedupe.Record{RunID: run, State: dedupe.StatePending}, nil
}

func (d *countingDedupe) Confirm(context.Context, dedupe.Key, domain.RunID, int64, time.Duration, time.Time) error {
	d.uses++
	return nil
}

func (d *countingDedupe) Release(context.Context, dedupe.Key, domain.RunID) error {
	d.uses++
	return nil
}

// dedupedLookup declares a semantic dedupe on the read it is asked about, so a real run
// would consult the store before calling it.
type dedupedLookup struct{}

func (dedupedLookup) Scope(domain.ToolID) (domain.Scope, bool) {
	return domain.Scope{Company: domain.Installation}, true
}
func (dedupedLookup) Effect(domain.ToolID) (domain.Effect, bool) { return domain.EffectRead, true }
func (dedupedLookup) Dedupe(domain.ToolID) (domain.ToolDedupe, bool) {
	return domain.ToolDedupe{WindowSeconds: 3600, ArgPaths: []string{"id"}}, true
}

type oneLookup struct{ done bool }

func (p *oneLookup) Plan(context.Context, engine.PlanInput) (engine.Proposal, error) {
	if p.done {
		return engine.Proposal{Done: true, Outcome: "completed"}, nil
	}
	p.done = true
	return engine.Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"c-1"}`)}, nil
}

/*
A simulated run touches none of the dedupe store real runs share (NT-018 phase 5, C3). Its
calls reach nothing, so a record it confirmed would answer a real run's call as a duplicate of
an effect that never happened, and a reservation it held would make a real run wait on it.
*/
func TestSimulatedRun_aDedupedCall_touchesNoSharedDedupe(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	if _, err := l.Append(ctx, domain.Step{
		RunID: "run-sim", Kind: domain.StepRunStarted, Scope: scope, AgentID: "triage",
		Payload: []byte(`{"trigger":"simulation","simulated":true,"simulation":"sim-1"}`),
	}); err != nil {
		t.Fatalf("open the simulated run: %v", err)
	}
	shared := &countingDedupe{}
	runner := engine.NewRunner(simulate.Deps(engine.Deps{
		Ledger: l, Gate: gate.New(), Planner: &oneLookup{}, Tools: &liveTools{},
		Catalog: dedupedLookup{}, Content: engine.NewMemoryContent(), Clock: fixedClock{},
		Dedupe: shared, DedupePendingWait: 20 * time.Millisecond, DedupePendingPoll: time.Millisecond,
	}))
	if _, err := runner.Advance(ctx, engine.Start{
		RunID: "run-sim", Scope: scope, AgentID: "triage", VersionID: "v1",
		Pack: gate.NewPack("crm.lookup"), OnBehalfOf: "ana", Trigger: "simulation",
		Budget: domain.Budget{Micros: 1_000_000, ToolCalls: 10, Steps: 50},
	}); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if shared.uses != 0 {
		t.Errorf("the simulation used the shared dedupe store %d times; want none", shared.uses)
	}
	steps, _ := l.Read(ctx, "run-sim", domain.FirstSeq)
	if !lookedUp(steps) {
		t.Fatal("the lookup was never called: the test would pass without reaching the store")
	}
}

// lookedUp is whether the run made its call, so the store was there to be reached.
func lookedUp(steps []domain.Step) bool {
	for _, s := range steps {
		if s.Kind == domain.StepToolCalled {
			return true
		}
	}
	return false
}
