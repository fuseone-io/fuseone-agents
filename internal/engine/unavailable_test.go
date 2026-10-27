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

// holding answers which server owns a tool on this worker and whether that
// server's session is live. Ownership is per tool, as registration makes it.
type holding struct {
	owner map[domain.ToolID]string
	live  map[string]bool
}

func (h holding) Session(id domain.ToolID) (string, bool, bool) {
	server, owned := h.owner[id]
	return server, h.live[server], owned
}

func (h holding) Connected(server string) bool { return h.live[server] }

// crmHeld registers the three crm envelope tools under the crm server.
func crmHeld(live bool) holding {
	return holding{
		owner: map[domain.ToolID]string{
			"crm.lookup": "crm", "crm.note": "crm", "crm.refund": "crm",
		},
		live: map[string]bool{"crm": live},
	}
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
	h.runner.deps.Connections = crmHeld(false)
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
	// The parked payload keeps the summary and drops the error text, so the
	// server must ride in it or the park names nobody.
	if summary.Provider != "crm" {
		t.Errorf("summary.Provider = %q, want the server the park is about", summary.Provider)
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
	h.runner.deps.Connections = holding{}
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
	h.runner.deps.Connections = crmHeld(true)
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
	h.runner.deps.Connections = crmHeld(true)
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

// The catalogue deliberately keeps a dead session's entries until the
// reconciler replaces them (tools/dead_session_test.go), so schema presence
// cannot answer whether the server is reachable. In that window every call
// would fail at invoke; the turn must not start.
func TestPlan_aDeadSessionWhoseEntriesAreStillCatalogued_parksBeforeTheModel(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	planner := &capturingPlanner{}
	h.runner.deps.Planner = planner
	h.runner.deps.Schemas = describing{"crm.lookup": true, "crm.note": true, "crm.refund": true}
	h.runner.deps.Connections = crmHeld(false)
	h.runner.deps.Servers = &expecting{servers: map[string]bool{"crm": true}}

	_, err := h.runner.Advance(context.Background(), h.start(t, generousBudget()))

	var unavailable *ToolServerUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("Advance err = %v, want ToolServerUnavailableError", err)
	}
	if unavailable.Server != "crm" || len(unavailable.Tools) != 3 {
		t.Errorf("named %q with %d tools, want crm with the whole envelope share",
			unavailable.Server, len(unavailable.Tools))
	}
	if len(planner.inputs) != 0 {
		t.Error("the model was called over a dead session")
	}
}

// A tool taken off the surface of a live, configured server is a Curator's
// decision, not an outage: the run proceeds and the absence is stated.
func TestPlan_aToolOffTheSurfaceOfALiveServer_isANoteNotAnOutage(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	planner := &capturingPlanner{}
	h.runner.deps.Planner = planner
	h.runner.deps.Schemas = describing{"crm.lookup": true, "crm.note": true}
	h.runner.deps.Connections = crmHeld(true)
	h.runner.deps.Servers = &expecting{servers: map[string]bool{"crm": true}}

	if _, err := h.runner.Advance(context.Background(), h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if len(planner.inputs) == 0 {
		t.Fatal("the model was never called")
	}
	got := planner.inputs[0].Unavailable
	if len(got) != 1 || got[0] != "crm.refund" {
		t.Errorf("Unavailable = %v, want the off-surface tool as a note", got)
	}
}

func TestPlan_twoServersMissing_theExpectedOneStopsTheTurn(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	planner := &capturingPlanner{}
	h.runner.deps.Planner = planner
	// crm.refund is missing from a removed server; gw.list from one that is
	// configured and enabled but disconnected. The outage wins: proceeding
	// would plan a run that cannot do the part somebody still expects it to.
	h.runner.deps.Schemas = describing{"crm.lookup": true, "crm.note": true}
	h.runner.deps.Connections = holding{}
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

// The reviewer's reproduction on #150 round 3: a fully described native
// connector tool must not park the run because an MCP server that happens
// to share its prefix is configured and disconnected. Ownership is declared
// by registration, never inferred from the ID's shape — and a namespace the
// catalogue owns nothing under must not cost a settings read per turn.
func TestPlan_aNativeToolSharingADisconnectedServersPrefix_runsUntouched(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	planner := &capturingPlanner{}
	servers := &expecting{servers: map[string]bool{"sql": true}}
	h.runner.deps.Planner = planner
	h.runner.deps.Schemas = describing{
		"crm.lookup": true, "crm.note": true, "crm.refund": true,
		"sql.app-x.run_query_template": true,
	}
	h.runner.deps.Connections = crmHeld(true)
	h.runner.deps.Servers = servers

	start := h.start(t, generousBudget())
	start.Pack = gate.NewPack("crm.lookup", "crm.note", "crm.refund", "sql.app-x.run_query_template")

	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if len(planner.inputs) == 0 {
		t.Fatal("the model was never called")
	}
	if got := planner.inputs[0].Unavailable; got != nil {
		t.Errorf("Unavailable = %v, want none: the native tool is described and owned by nobody's session", got)
	}
	if servers.asks != 0 {
		t.Errorf("settings were read %d times for a namespace the catalogue does not own", servers.asks)
	}
}

// A nudge that fails after a billed call appends a planned step for the
// spend. That prompt went out without part of the pack too, and a spent
// step that does not say so leaves the trail mute exactly where the money
// went (#150 round 3).
func TestPlan_aSpentFailure_stillRecordsWhatThePromptCouldNotOffer(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.runner.deps.Planner = &spendingPlanner{err: &SpentPlanError{
		Cost:     domain.Cost{Micros: 1200, InputTokens: 300},
		Provider: "anthropic",
		Model:    "claude-opus-5",
		Err:      context.DeadlineExceeded,
	}}
	h.runner.deps.Schemas = describing{"crm.lookup": true, "crm.note": true}
	h.runner.deps.Connections = holding{}
	h.runner.deps.Servers = &expecting{servers: map[string]bool{}}

	if _, err := h.runner.Advance(context.Background(), h.start(t, generousBudget())); err == nil {
		t.Fatal("Advance swallowed the planner failure")
	}

	var p domain.PlannedPayload
	if err := h.lastPayloadOf(t, domain.StepPlanned, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if len(p.UnavailableTools) != 1 || p.UnavailableTools[0] != "crm.refund" {
		t.Errorf("recorded %v, want the billed prompt's missing tool", p.UnavailableTools)
	}
}

// spendingPlanner fails every plan after having billed a call.
type spendingPlanner struct{ err *SpentPlanError }

func (p *spendingPlanner) Plan(context.Context, PlanInput) (Proposal, error) {
	return Proposal{}, p.err
}

// A live server that merely stopped offering a tool after a re-list is the
// Curator's — or the server's — decision: the entry is gone, so nobody owns
// the ID, but the prefix's server is alive and answering. Parking that as a
// disconnection would hold the run for ever over an evolution (#150 round 4).
func TestPlan_aLiveServerThatStoppedOfferingATool_isANoteNotAnOutage(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	planner := &capturingPlanner{}
	servers := &expecting{servers: map[string]bool{"crm": true}}
	h.runner.deps.Planner = planner
	h.runner.deps.Schemas = describing{"crm.lookup": true, "crm.note": true}
	// refund's entry is gone from the re-listed catalogue — owned by nobody —
	// while the crm session is alive.
	h.runner.deps.Connections = holding{
		owner: map[domain.ToolID]string{"crm.lookup": "crm", "crm.note": "crm"},
		live:  map[string]bool{"crm": true},
	}
	h.runner.deps.Servers = servers

	if _, err := h.runner.Advance(context.Background(), h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if len(planner.inputs) == 0 {
		t.Fatal("the model was never called")
	}
	got := planner.inputs[0].Unavailable
	if len(got) != 1 || got[0] != "crm.refund" {
		t.Errorf("Unavailable = %v, want the dropped tool as a note", got)
	}
	if servers.asks != 0 {
		t.Errorf("settings were read %d times for a live server", servers.asks)
	}
}
