package simulate_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
)

// coveringGrant is a standing grant that covers every call it is asked about,
// and counts the daily uses it was asked to spend.
type coveringGrant struct{ claims int }

func (g *coveringGrant) Claim(
	context.Context, domain.ToolID, domain.AgentID, domain.Scope, domain.RunID, time.Time,
) (engine.StandingGrant, bool, error) {
	g.claims++
	return engine.StandingGrant{ID: "SG-7", By: "usr_ana", Reason: "night shift"}, true, nil
}

type oneNote struct{ done bool }

func (p *oneNote) Plan(context.Context, engine.PlanInput) (engine.Proposal, error) {
	if p.done {
		return engine.Proposal{Done: true, Outcome: "completed"}, nil
	}
	p.done = true
	return engine.Proposal{Tool: "crm.note", Args: []byte(`{"text":"hi"}`)}, nil
}

type writeCatalog struct{}

func (writeCatalog) Scope(domain.ToolID) (domain.Scope, bool) {
	return domain.Scope{Company: domain.Installation}, true
}
func (writeCatalog) Effect(domain.ToolID) (domain.Effect, bool) { return domain.EffectWrite, true }
func (writeCatalog) Dedupe(domain.ToolID) (domain.ToolDedupe, bool) {
	return domain.ToolDedupe{}, false
}

/*
A simulated run parks for a person where a standing grant would have released
the real one, and the grant is never asked. Asking spends one of its daily
uses, and writes a real approval in the owner's name: a battery of fifty cases
could use up the day's releases, and the production runs after it would wait
for somebody who was promised they would not have to.

The Gate's verdict is recorded either way, so a battery that expects an
approval still reads one.
*/
func TestSimulatedRun_aStandingGrantCoversTheWrite_parksWithoutClaimingIt(t *testing.T) {
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
	grant := &coveringGrant{}
	runner := engine.NewRunner(simulate.Deps(engine.Deps{
		Ledger: l, Gate: gate.New(), Planner: &oneNote{}, Tools: &liveTools{},
		Catalog: writeCatalog{}, Content: engine.NewMemoryContent(), Clock: fixedClock{},
		Standing: grant,
	}))
	st, err := runner.Advance(ctx, engine.Start{
		RunID: "run-sim", Scope: scope, AgentID: "triage", VersionID: "v1",
		Pack: gate.NewPack("crm.note"), OnBehalfOf: "ana", Trigger: "simulation",
		Budget: domain.Budget{Micros: 1_000_000, ToolCalls: 10, Steps: 50},
	})
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if grant.claims != 0 {
		t.Errorf("the simulation asked the grant %d times; each is a daily use spent", grant.claims)
	}
	if st.Phase != engine.PhaseAwaitingApproval {
		t.Errorf("phase = %v, want the simulated run parked for a person", st.Phase)
	}
	steps, _ := l.Read(ctx, "run-sim", domain.FirstSeq)
	for _, s := range steps {
		if s.Kind == domain.StepApprovalDecided {
			var p domain.ApprovalDecidedPayload
			_ = json.Unmarshal(s.Payload, &p)
			t.Errorf("a simulated run recorded a real approval: %+v", p)
		}
	}
}
