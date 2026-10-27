package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

// A worker whose catalogue cannot describe a pack tool used to plan the run
// anyway: the model was never offered the tool, the Gate never saw a
// proposal, and the run finished "clean" having been unable to do its work.
// These tests pin the honest behaviour (#148).

// describing answers the schema lookup for exactly the IDs it was given.
type describing map[domain.ToolID]bool

func (d describing) Schema(id domain.ToolID) (string, string, map[string]any, bool) {
	if !d[id] {
		return "", "", nil, false
	}
	return string(id), "described", map[string]any{}, true
}

// expecting answers which servers are configured and enabled, counting asks.
type expecting struct {
	servers map[string]bool
	asks    int
}

func (e *expecting) Expected(_ context.Context, server string) (bool, error) {
	e.asks++
	return e.servers[server], nil
}

// capturingPlanner records every PlanInput and finishes the run.
type capturingPlanner struct {
	inputs []PlanInput
}

func (p *capturingPlanner) Plan(_ context.Context, in PlanInput) (Proposal, error) {
	p.inputs = append(p.inputs, in)
	return Proposal{Done: true, Outcome: "completed"}, nil
}

func TestPlan_aPackToolWhoseExpectedServerIsGone_failsRetryableBeforeTheModel(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	planner := &capturingPlanner{}
	h.runner.deps.Planner = planner
	h.runner.deps.Schemas = describing{"crm.lookup": true, "crm.note": true}
	h.runner.deps.Servers = &expecting{servers: map[string]bool{"crm": true}}

	_, err := h.runner.Advance(context.Background(), h.start(t, generousBudget()))

	var unavailable *ToolServerUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("Advance err = %v, want ToolServerUnavailableError", err)
	}
	if unavailable.Server != "crm" {
		t.Errorf("server = %q, want crm", unavailable.Server)
	}
	summary := unavailable.Summary()
	if summary.Code != CodeToolServerUnavailable || !summary.Retryable {
		t.Errorf("summary = %+v, want retryable %s", summary, CodeToolServerUnavailable)
	}
	if len(planner.inputs) != 0 {
		t.Errorf("the model was called %d times; a toolless turn must spend nothing", len(planner.inputs))
	}
}

func TestPlan_aPackToolOfARemovedServer_isNamedToTheModelAndRecorded(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	planner := &capturingPlanner{}
	h.runner.deps.Planner = planner
	h.runner.deps.Schemas = describing{"crm.lookup": true, "crm.note": true}
	// Nothing expects the server: it was removed or disabled on purpose, so
	// the run proceeds — but saying so, instead of leaving the model to
	// improvise an explanation.
	h.runner.deps.Servers = &expecting{servers: map[string]bool{}}

	if _, err := h.runner.Advance(context.Background(), h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	if len(planner.inputs) == 0 {
		t.Fatal("the model was never called")
	}
	got := planner.inputs[0].Unavailable
	if len(got) != 1 || got[0] != "crm.refund" {
		t.Errorf("Unavailable = %v, want the one undescribable tool", got)
	}

	var p domain.PlannedPayload
	if err := h.payloadOf(t, domain.StepPlanned, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if len(p.UnavailableTools) != 1 || p.UnavailableTools[0] != "crm.refund" {
		t.Errorf("recorded %v, want the trail to name what could not be offered", p.UnavailableTools)
	}
}

func TestPlan_nothingMissing_neverAsksWhichServersAreExpected(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	planner := &capturingPlanner{}
	servers := &expecting{servers: map[string]bool{"crm": true}}
	h.runner.deps.Planner = planner
	h.runner.deps.Schemas = describing{"crm.lookup": true, "crm.note": true, "crm.refund": true}
	h.runner.deps.Servers = servers

	if _, err := h.runner.Advance(context.Background(), h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if servers.asks != 0 {
		t.Errorf("asked about servers %d times with a fully described pack", servers.asks)
	}
	if len(planner.inputs) == 0 || planner.inputs[0].Unavailable != nil {
		t.Errorf("Unavailable = %v, want none", planner.inputs[0].Unavailable)
	}
}

func TestPlan_platformToolsAreNeverReportedUnavailable(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	planner := &capturingPlanner{}
	h.runner.deps.Planner = planner
	// The schemas port describes nothing platform-owned: memory tools answer
	// through the memory layer in production, and the check must not read
	// that as an outage of a server called "$fuseone".
	h.runner.deps.Schemas = describing{"crm.lookup": true, "crm.note": true, "crm.refund": true}
	h.runner.deps.Servers = &expecting{servers: map[string]bool{}}

	start := h.start(t, generousBudget())
	start.MemoryLearning = domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview}

	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if len(planner.inputs) == 0 {
		t.Fatal("the model was never called")
	}
	if got := planner.inputs[0].Unavailable; got != nil {
		t.Errorf("Unavailable = %v, want none: platform tools are always servable", got)
	}
}

func TestPlan_withNoSchemasPort_plansExactlyAsBefore(t *testing.T) {
	t.Parallel()

	// The check is opt-in by wiring. An installation that gave the engine no
	// schema view — the in-memory test rig is one — keeps today's behaviour.
	h := newHarness(t)
	planner := &capturingPlanner{}
	h.runner.deps.Planner = planner
	h.runner.deps.Servers = &expecting{servers: map[string]bool{"crm": true}}

	if _, err := h.runner.Advance(context.Background(), h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if len(planner.inputs) == 0 || planner.inputs[0].Unavailable != nil {
		t.Errorf("Unavailable = %v, want none without a schema view", planner.inputs[0].Unavailable)
	}
}

func TestPlan_twoServersMissing_theExpectedOneStopsTheTurn(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	planner := &capturingPlanner{}
	h.runner.deps.Planner = planner
	// crm.refund is missing from a removed server; gw.list from one that is
	// configured and enabled. The outage wins: proceeding would plan a run
	// that cannot do the part somebody still expects it to do.
	h.runner.deps.Schemas = describing{"crm.lookup": true, "crm.note": true}
	h.runner.deps.Servers = &expecting{servers: map[string]bool{"gw": true}}

	start := h.start(t, generousBudget())
	start.Pack = gate.NewPack("crm.lookup", "crm.note", "crm.refund", "gw.list")

	_, err := h.runner.Advance(context.Background(), start)

	var unavailable *ToolServerUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("Advance err = %v, want ToolServerUnavailableError", err)
	}
	if unavailable.Server != "gw" {
		t.Errorf("server = %q, want the expected one, gw", unavailable.Server)
	}
	if len(planner.inputs) != 0 {
		t.Error("the model was called despite the outage")
	}
}
