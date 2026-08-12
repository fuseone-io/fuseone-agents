package simulate_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
	"github.com/fuseone/agents/internal/trigger"
)

type registry struct{}

func (registry) Versions(context.Context, domain.AgentID) ([]domain.AgentSummary, error) {
	return []domain.AgentSummary{{
		ID: "suporte", VersionID: "v1", Latest: true,
		Scope: domain.Scope{Company: "acme", Area: "cx"},
	}}, nil
}

type catalog map[domain.ToolID]domain.Effect

func (c catalog) Effect(id domain.ToolID) (domain.Effect, bool) {
	e, ok := c[id]
	return e, ok
}

type clock struct{}

func (clock) Now() time.Time { return time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC) }

// liveTools is a tool layer that acts. Nothing in a simulation may reach it,
// and this is here to prove nothing does.
type liveTools struct{ calls int }

func (l *liveTools) Invoke(context.Context, engine.Call) (engine.ToolResult, error) {
	l.calls++
	return engine.ToolResult{}, nil
}

// onceThenDone proposes one call per run and then reports the run complete.
type onceThenDone struct{ tool domain.ToolID }

func (p onceThenDone) Plan(_ context.Context, in engine.PlanInput) (engine.Proposal, error) {
	if len(in.State.Called) == 0 {
		return engine.Proposal{Tool: p.tool, Args: []byte(`{}`)}, nil
	}
	return engine.Proposal{Done: true, Outcome: "respondido"}, nil
}

type failsOn struct {
	inner *trigger.Opener
	at    int
	seen  int
}

func (f *failsOn) Open(ctx context.Context, req trigger.Request) (trigger.Result, error) {
	f.seen++
	if f.seen == f.at {
		return trigger.Result{}, errors.New("trigger: the agent is paused")
	}
	return f.inner.Open(ctx, req)
}

func openerFor(store *ledger.Memory, content engine.ContentStore) *trigger.Opener {
	return trigger.NewOpener(store, registry{}, clock{}).WithContent(content)
}

func depsFor(store *ledger.Memory, content *engine.MemoryContent, tools engine.Tools) engine.Deps {
	return engine.Deps{
		Ledger: store, Content: content, Gate: gate.New(), Tools: tools,
		Catalog: catalog{"crm.lookup": domain.EffectRead, "crm.refund": domain.EffectFinancial},
		Clock:   clock{},
	}
}

func jobFor(tool domain.ToolID, cases ...string) simulate.Job {
	inputs := make([][]byte, 0, len(cases))
	for _, c := range cases {
		inputs = append(inputs, []byte(c))
	}
	return simulate.Job{
		ID: "sim-1", Agent: "suporte", Version: "v1",
		Start: engine.Start{
			Pack:   gate.NewPack("crm.lookup"),
			Budget: domain.Budget{Micros: 1_000_000, Steps: 40},
		},
		Planner: onceThenDone{tool: tool},
		Cases:   inputs,
	}
}

func TestRun_neverReachesTheToolLayerItWasBuiltWith(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	live := &liveTools{}
	exec := simulate.NewExecutor(openerFor(store, content), depsFor(store, content, live))

	report, err := exec.Run(t.Context(), jobFor("crm.lookup", `{"assunto":"cobrança"}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The property the whole feature rests on: a run marked simulated has no
	// path to a real system, whatever the caller handed the constructor.
	if live.calls != 0 {
		t.Fatalf("the real tool layer was invoked %d times", live.calls)
	}
	// And it got as far as the tool layer, so the absence above is the dry
	// layer standing in rather than the run never having proposed anything.
	if len(report.Cases) != 1 || len(report.Cases[0].Acted) != 1 {
		t.Fatalf("report = %+v", report.Cases)
	}
	if act := report.Cases[0].Acted[0]; !act.Reached || act.Tool != "crm.lookup" {
		t.Errorf("act = %+v", act)
	}
	if report.Cases[0].Settled != simulate.SettledFinished {
		t.Errorf("settled = %q", report.Cases[0].Settled)
	}
}

func TestRun_opensOneRunPerCase_eachMarkedSimulated(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	exec := simulate.NewExecutor(openerFor(store, content), depsFor(store, content, &liveTools{}))

	report, err := exec.Run(t.Context(), jobFor("crm.lookup", `{"n":1}`, `{"n":2}`, `{"n":3}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.Cases) != 3 {
		t.Fatalf("cases = %d", len(report.Cases))
	}

	seen := map[domain.RunID]bool{}
	for i, c := range report.Cases {
		if seen[c.RunID] {
			// Three cases sharing a run is three cases reported as one.
			t.Fatalf("case %d reuses run %q", i+1, c.RunID)
		}
		seen[c.RunID] = true

		steps, err := store.Read(t.Context(), c.RunID, domain.FirstSeq)
		if err != nil {
			t.Fatalf("read %s: %v", c.RunID, err)
		}
		var p domain.RunStartedPayload
		if err := json.Unmarshal(steps[0].Payload, &p); err != nil {
			t.Fatalf("payload: %v", err)
		}
		// The mark travels in the step, so every projection excludes it
		// without being told separately.
		if !p.Simulated || p.Trigger != "simulation" {
			t.Errorf("run %s opened as %+v", c.RunID, p)
		}
	}
}

func TestRun_aCaseThatCannotOpen_isStillARowInTheReport(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	opener := &failsOn{inner: openerFor(store, content), at: 2}
	exec := simulate.NewExecutor(opener, depsFor(store, content, &liveTools{}))

	report, err := exec.Run(t.Context(), jobFor("crm.lookup", `{"n":1}`, `{"n":2}`, `{"n":3}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Reporting two of three and mentioning nothing would give an author
	// coverage that is a lie — the same reason Load refuses a bad line.
	if len(report.Cases) != 3 {
		t.Fatalf("cases = %d, want every case accounted for", len(report.Cases))
	}
	if report.Cases[1].Error == "" {
		t.Error("the case that could not open reports no error")
	}
	if report.Cases[0].Settled != simulate.SettledFinished ||
		report.Cases[2].Settled != simulate.SettledFinished {
		t.Error("one case failing stopped the others")
	}
}

func TestRun_reportsWhereTheGateWouldHaveStopped(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	exec := simulate.NewExecutor(openerFor(store, content), depsFor(store, content, &liveTools{}))

	// Outside the pack the version was frozen with: the Gate refuses it on
	// capability, and this is the row the author most needs to see.
	report, err := exec.Run(t.Context(), jobFor("crm.refund", `{"assunto":"estorno"}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := report.Cases[0]
	if len(got.Acted) == 0 {
		t.Fatalf("no act recorded: %+v", got)
	}
	if got.Acted[0].Verdict != domain.VerdictBlock || got.Acted[0].Rule != gate.RuleCapability {
		t.Errorf("act = %+v, want a capability block", got.Acted[0])
	}
	if got.Acted[0].Reached {
		t.Error("a blocked proposal is reported as having reached the tool layer")
	}
}

func TestRun_withoutAPlanner_refusesRatherThanPanics(t *testing.T) {
	t.Parallel()

	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	exec := simulate.NewExecutor(openerFor(store, content), depsFor(store, content, &liveTools{}))

	job := jobFor("crm.lookup", `{"n":1}`)
	job.Planner = nil
	// A misconfiguration must not open runs first and fall over on the first
	// turn, leaving half a simulation in the ledger.
	if _, err := exec.Run(t.Context(), job); err == nil {
		t.Fatal("want a refusal")
	}
}
