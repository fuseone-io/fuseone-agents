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

type oneClaim struct{ done bool }

func (p *oneClaim) Plan(context.Context, engine.PlanInput) (engine.Proposal, error) {
	if p.done {
		return engine.Proposal{Done: true, Outcome: "completed"}, nil
	}
	p.done = true
	return engine.Proposal{
		Tool: domain.ToolWorkItemClaim,
		Args: []byte(`{"ref":"github:fuseone-io/fuseone-agents#123"}`),
	}, nil
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }

// The claim is executed by the engine, not by the tool layer this package
// replaces — so the dry boundary has to hold for it too. A simulated run sees
// the claim succeed, and a real run of the same agent can still take the card.
func TestSimulatedRun_workItemClaim_succeedsWithoutHoldingTheCard(t *testing.T) {
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
	runner := engine.NewRunner(simulate.Deps(engine.Deps{
		Ledger: l, Gate: gate.New(), Planner: &oneClaim{}, Tools: &liveTools{},
		Catalog: simulateCatalog{}, Content: engine.NewMemoryContent(), Clock: fixedClock{},
	}))
	start := engine.Start{
		RunID: "run-sim", Scope: scope, AgentID: "triage", VersionID: "v1",
		Pack: gate.NewPack(domain.ToolWorkItemClaim), Stage: domain.StageAutonomous,
		Budget: domain.Budget{Micros: 1_000_000, ToolCalls: 10, Steps: 50},
	}
	if _, err := runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if returned := lastReturned(t, l); returned.Failed {
		t.Fatalf("simulated claim result = %+v, want the rehearsal to succeed", returned)
	}

	if _, err := l.Append(ctx, domain.Step{
		RunID: "run-real", Kind: domain.StepRunStarted, Scope: scope, AgentID: "triage",
		Payload: []byte(`{"trigger":"cron"}`),
	}); err != nil {
		t.Fatalf("open the real run: %v", err)
	}
	if _, err := l.Append(ctx, domain.Step{
		RunID: "run-real", Kind: domain.StepWorkItemClaimed, Scope: scope, AgentID: "triage",
		Payload: []byte(`{"ref":"github:fuseone-io/fuseone-agents#123"}`),
	}); err != nil {
		t.Fatalf("a simulated run acquired a durable production work-item claim: %v", err)
	}
}

type simulateCatalog struct{}

func (simulateCatalog) Effect(domain.ToolID) (domain.Effect, bool) {
	return domain.EffectUnknown, false
}

func (simulateCatalog) Dedupe(domain.ToolID) (domain.ToolDedupe, bool) {
	return domain.ToolDedupe{}, false
}

func lastReturned(t *testing.T, l *ledger.Memory) domain.ToolReturnedPayload {
	t.Helper()
	steps, err := l.Read(context.Background(), "run-sim", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Kind == domain.StepToolReturned {
			var p domain.ToolReturnedPayload
			if err := json.Unmarshal(steps[i].Payload, &p); err != nil {
				t.Fatalf("decode: %v", err)
			}
			return p
		}
	}
	t.Fatal("the simulated run never returned from its claim")
	return domain.ToolReturnedPayload{}
}
