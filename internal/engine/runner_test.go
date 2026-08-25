package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	effectdedupe "github.com/fuseone/agents/internal/dedupe"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
)

// Both ledger implementations must satisfy the port this package declares.
// The assertion lives here, with the consumer: an implementation that imported
// engine to prove it complies would create a cycle the moment engine's tests
// used it.
var (
	_ Ledger = (*ledger.Memory)(nil)
	_ Ledger = (*ledger.Postgres)(nil)
)

// --- fakes -----------------------------------------------------------------

// scriptedPlanner returns one proposal per call, then reports the run done.
type scriptedPlanner struct {
	proposals []Proposal
	calls     int
}

func (p *scriptedPlanner) Plan(context.Context, PlanInput) (Proposal, error) {
	p.calls++
	if p.calls > len(p.proposals) {
		return Proposal{Done: true, Outcome: "completed"}, nil
	}
	return p.proposals[p.calls-1], nil
}

// countingTools records every invocation so a test can prove an effect
// happened exactly once.
//
// It stores its result the way a real adapter does — the reference it returns
// resolves — because a fake that hands back a dangling reference lets a
// transcript bug pass here and fail in production.
type countingTools struct {
	invocations []domain.ToolID
	calls       []Call
	content     ContentStore
	body        []byte
	reserveErr  error
	err         error
	failed      bool
	errorCode   string
}

func (c *countingTools) Reserve(context.Context, Call) error {
	return c.reserveErr
}

func (c *countingTools) Invoke(ctx context.Context, call Call) (ToolResult, error) {
	c.invocations = append(c.invocations, call.Tool)
	c.calls = append(c.calls, call)
	result := ToolResult{Failed: c.failed, ErrorCode: c.errorCode}
	if len(c.body) > 0 {
		ref, err := c.content.Put(ctx, call.RunID, call.Seq, c.body)
		if err != nil {
			return ToolResult{}, err
		}
		result.ResultRef = ref
	}
	if c.err != nil {
		return result, c.err
	}
	return result, nil
}

type staticCatalog map[domain.ToolID]domain.Effect

func (s staticCatalog) Effect(id domain.ToolID) (domain.Effect, bool) {
	e, ok := s[id]
	return e, ok
}

func (s staticCatalog) Dedupe(domain.ToolID) (domain.ToolDedupe, bool) {
	return domain.ToolDedupe{}, false
}

type dedupeCatalog struct {
	staticCatalog
	dedupes map[domain.ToolID]domain.ToolDedupe
}

func (c dedupeCatalog) Dedupe(id domain.ToolID) (domain.ToolDedupe, bool) {
	d, ok := c.dedupes[id]
	return d.Clone(), ok && d.Enabled()
}

type fakeDedupeStore struct {
	mu sync.Mutex

	lookupFound    []bool
	lookupRecords  []effectdedupe.Record
	lookupErr      error
	reserveRecords []effectdedupe.Record
	reserveErr     error
	confirmErr     error
	releaseErr     error

	lookups  []effectdedupe.Key
	reserves []effectdedupe.Key
	confirms []dedupeConfirm
	releases []dedupeRelease
}

type dedupeConfirm struct {
	key effectdedupe.Key
	run domain.RunID
	seq int64
}

type dedupeRelease struct {
	key effectdedupe.Key
	run domain.RunID
}

func (f *fakeDedupeStore) Lookup(
	_ context.Context, key effectdedupe.Key, _ time.Time,
) (effectdedupe.Record, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups = append(f.lookups, key)
	if f.lookupErr != nil {
		return effectdedupe.Record{}, false, f.lookupErr
	}
	if len(f.lookupFound) == 0 {
		return effectdedupe.Record{}, false, nil
	}
	found := f.lookupFound[0]
	f.lookupFound = f.lookupFound[1:]
	var rec effectdedupe.Record
	if len(f.lookupRecords) > 0 {
		rec = f.lookupRecords[0]
		f.lookupRecords = f.lookupRecords[1:]
	}
	return rec, found, nil
}

func (f *fakeDedupeStore) Reserve(
	_ context.Context, key effectdedupe.Key, run domain.RunID, _ time.Duration, _ time.Time,
) (effectdedupe.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reserves = append(f.reserves, key)
	if f.reserveErr != nil {
		return effectdedupe.Record{}, f.reserveErr
	}
	if len(f.reserveRecords) == 0 {
		return effectdedupe.Record{State: effectdedupe.StateReserved, RunID: run}, nil
	}
	rec := f.reserveRecords[0]
	f.reserveRecords = f.reserveRecords[1:]
	return rec, nil
}

func (f *fakeDedupeStore) Confirm(
	_ context.Context, key effectdedupe.Key, run domain.RunID, seq int64, _ time.Duration, _ time.Time,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.confirms = append(f.confirms, dedupeConfirm{key: key, run: run, seq: seq})
	return f.confirmErr
}

func (f *fakeDedupeStore) Release(
	_ context.Context, key effectdedupe.Key, run domain.RunID,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releases = append(f.releases, dedupeRelease{key: key, run: run})
	return f.releaseErr
}

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

// tickingClock advances a second every time it is read, so a test can watch a
// run age. Deterministic, unlike the wall clock, which is the whole reason the
// engine takes a Clock rather than calling time.Now.
type tickingClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *tickingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(time.Second)
	return c.t
}

// --- harness ---------------------------------------------------------------

type harness struct {
	runner  *Runner
	ledger  *ledger.Memory
	planner *scriptedPlanner
	tools   *countingTools
	content *MemoryContent
}

// payloadOf decodes the payload of the first step of a kind.
func (h *harness) payloadOf(t *testing.T, kind domain.StepKind, into any) error {
	t.Helper()
	step, err := h.stepOf(t, kind)
	if err != nil {
		return err
	}
	return json.Unmarshal(step.Payload, into)
}

func (h *harness) stepOf(t *testing.T, kind domain.StepKind) (domain.Step, error) {
	t.Helper()
	steps, err := h.ledger.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	i := slices.IndexFunc(steps, func(s domain.Step) bool { return s.Kind == kind })
	if i < 0 {
		return domain.Step{}, fmt.Errorf("no %s step in %v", kind, h.kinds(t))
	}
	return steps[i], nil
}

// lastPayloadOf decodes the payload of the last step of a kind.
func (h *harness) lastPayloadOf(t *testing.T, kind domain.StepKind, into any) error {
	t.Helper()
	steps, err := h.ledger.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Kind == kind {
			return json.Unmarshal(steps[i].Payload, into)
		}
	}
	return fmt.Errorf("no %s step in %v", kind, h.kinds(t))
}

func newHarness(t *testing.T, proposals ...Proposal) *harness {
	t.Helper()
	return newHarnessOn(t, fixedClock{t: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)}, proposals...)
}

func newHarnessOn(t *testing.T, clock Clock, proposals ...Proposal) *harness {
	t.Helper()

	l := ledger.NewMemory()
	planner := &scriptedPlanner{proposals: proposals}
	content := NewMemoryContent()
	tools := &countingTools{content: content, body: []byte(`{"ok":true}`)}

	h := &harness{ledger: l, planner: planner, tools: tools, content: content}
	h.runner = NewRunner(Deps{
		Ledger:  l,
		Content: content,
		Gate:    gate.New(),
		Planner: planner,
		Tools:   tools,
		Catalog: staticCatalog{
			"crm.lookup":           domain.EffectRead,
			"crm.note":             domain.EffectWrite,
			"crm.refund":           domain.EffectFinancial,
			domain.ToolContextRead: domain.EffectRead,
		},
		Clock: clock,
	})
	return h
}

func enableDedupe(h *harness, store *fakeDedupeStore) {
	h.runner.deps.Catalog = dedupeCatalog{
		staticCatalog: staticCatalog{
			"crm.lookup":           domain.EffectRead,
			"crm.note":             domain.EffectWrite,
			"crm.refund":           domain.EffectFinancial,
			domain.ToolContextRead: domain.EffectRead,
		},
		dedupes: map[domain.ToolID]domain.ToolDedupe{
			"crm.lookup": {WindowSeconds: 3600, ArgPaths: []string{"id"}},
		},
	}
	h.runner.deps.Dedupe = store
	h.runner.deps.DedupePendingWait = 20 * time.Millisecond
	h.runner.deps.DedupePendingPoll = time.Millisecond
}

func (h *harness) start(t *testing.T, b domain.Budget) Start {
	t.Helper()
	return Start{
		RunID:      "run-1",
		Scope:      domain.Scope{Company: "acme", Area: "cx"},
		AgentID:    "triage",
		VersionID:  "v3",
		OnBehalfOf: "ana",
		Pack:       gate.NewPack("crm.lookup", "crm.note", "crm.refund"),
		Budget:     b,
		Trigger:    "cron",
	}
}

func (h *harness) kinds(t *testing.T) []domain.StepKind {
	t.Helper()
	steps, err := h.ledger.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	out := make([]domain.StepKind, len(steps))
	for i, s := range steps {
		out[i] = s.Kind
	}
	return out
}

func generousBudget() domain.Budget {
	return domain.Budget{Micros: 1_000_000, ToolCalls: 20, Steps: 50}
}

// --- tests -----------------------------------------------------------------

func TestAdvance_readTool_recordsFullGatedCycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	want := []domain.StepKind{
		domain.StepRunStarted,
		domain.StepPlanned,
		domain.StepGateDecided,
		domain.StepBudgetReserved,
		domain.StepToolCalled,
		domain.StepToolReturned,
		domain.StepBudgetReconciled,
	}
	got := h.kinds(t)
	if len(got) != len(want) {
		t.Fatalf("ledger = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d = %s, want %s", i, got[i], want[i])
		}
	}
	if err := h.ledger.Verify(ctx, "run-1"); err != nil {
		t.Errorf("Verify: %v", err)
	}
}

func TestAdvance_contextRead_isNotCapabilityWithoutAContract(t *testing.T) {
	t.Parallel()

	h := newHarness(t, Proposal{
		Tool: domain.ToolContextRead,
		Args: []byte(`{"name":"triage_summary"}`),
	})

	if _, err := h.runner.Advance(context.Background(), h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	if len(h.tools.invocations) != 0 {
		t.Fatalf("context read invoked without a contract: %v", h.tools.invocations)
	}
	var decided domain.GateDecidedPayload
	if err := h.payloadOf(t, domain.StepGateDecided, &decided); err != nil {
		t.Fatalf("gate payload: %v", err)
	}
	if decided.Rule != gate.RuleCapability {
		t.Fatalf("rule = %q, want capability", decided.Rule)
	}
}

func TestAdvance_contextRead_isCapabilityWhenRunStartedWithAContract(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	artifact := domain.ContextArtifact{
		Name: "triage_summary", Ref: "run://source/3/abc",
		Digest: "sha256:abc", SourceRun: "source",
		Labels: domain.NewLabels(domain.LabelUntrusted),
	}
	h := newHarness(t, Proposal{
		Tool: domain.ToolContextRead,
		Args: []byte(`{"name":"triage_summary"}`),
	})
	start := h.start(t, generousBudget())
	if _, err := h.ledger.Append(ctx, domain.Step{
		RunID: start.RunID, Kind: domain.StepRunStarted,
		Scope: start.Scope, AgentID: start.AgentID,
		VersionID: start.VersionID, OnBehalfOf: start.OnBehalfOf,
		Payload: mustJSON(domain.RunStartedPayload{
			Trigger: "event", ContextArtifacts: []domain.ContextArtifact{artifact},
		}),
	}); err != nil {
		t.Fatalf("open run: %v", err)
	}

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	if len(h.tools.calls) != 1 {
		t.Fatalf("tool calls = %d, want one", len(h.tools.calls))
	}
	if got := h.tools.calls[0].ContextArtifacts; len(got) != 1 || got[0].Name != "triage_summary" {
		t.Fatalf("context contract on call = %+v", got)
	}
	var decided domain.GateDecidedPayload
	if err := h.payloadOf(t, domain.StepGateDecided, &decided); err != nil {
		t.Fatalf("gate payload: %v", err)
	}
	if decided.Rule != gate.RulePassed || decided.Effect != domain.EffectRead {
		t.Fatalf("decision = %s/%s, want passed/read", decided.Rule, decided.Effect)
	}
}

func TestAdvance_recordsThePromptCompositionThePlannerMeasured(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	prompt := domain.PromptInputBreakdown{
		Unit:        "content_bytes",
		Input:       31,
		ToolResults: 1200,
		ToolResultsByTool: map[domain.ToolID]int64{
			"crm.lookup": 1200,
		},
		Total: 1231,
	}
	h := newHarness(t, Proposal{
		Tool:   "crm.lookup",
		Args:   []byte(`{"id":"42"}`),
		Prompt: prompt,
		Price: domain.ModelPriceUse{
			Status:         domain.ModelPriceConfigured,
			NonZeroApplied: true,
		},
	})

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var got domain.PlannedPayload
	if err := h.payloadOf(t, domain.StepPlanned, &got); err != nil {
		t.Fatalf("planned payload: %v", err)
	}
	if got.Prompt == nil {
		t.Fatal("Prompt = nil, want the planner's measured prompt composition")
	}
	if got.Prompt.Unit != "content_bytes" || got.Prompt.ToolResults != 1200 ||
		got.Prompt.ToolResultsByTool["crm.lookup"] != 1200 {
		t.Fatalf("Prompt = %+v, want the measured tool result bytes", *got.Prompt)
	}
	if got.Price == nil || got.Price.Status != domain.ModelPriceConfigured ||
		!got.Price.NonZeroApplied {
		t.Fatalf("Price = %+v, want configured non-zero rate provenance", got.Price)
	}
}

func TestAdvance_recordsModelAndToolInputLabelsOnTheTrail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	start := h.start(t, generousBudget())
	labels := domain.ScopeLabels(start.Scope).Union(domain.NewLabels(domain.LabelUntrusted))
	if _, err := h.ledger.Append(ctx, domain.Step{
		RunID: start.RunID, Kind: domain.StepRunStarted,
		Scope: start.Scope, AgentID: start.AgentID,
		VersionID: start.VersionID, OnBehalfOf: start.OnBehalfOf,
		Labels:  labels,
		Payload: mustJSON(domain.RunStartedPayload{Trigger: "webhook"}),
	}); err != nil {
		t.Fatalf("open run: %v", err)
	}

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	for _, kind := range []domain.StepKind{
		domain.StepPlanned,
		domain.StepGateDecided,
		domain.StepToolCalled,
	} {
		step, err := h.stepOf(t, kind)
		if err != nil {
			t.Fatal(err)
		}
		if !step.Labels.Has(domain.LabelUntrusted) ||
			!step.Labels.Has(domain.LabelArea(start.Scope)) {
			t.Fatalf("%s labels = %v, want model/tool input provenance", kind, step.Labels)
		}
	}
}

func TestAdvance_recordsApprovalInputLabelsOnTheTrail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.note", Args: []byte(`{"text":"hi"}`)})
	start := h.start(t, generousBudget())
	labels := domain.ScopeLabels(start.Scope).Union(domain.NewLabels(domain.LabelUntrusted))
	if _, err := h.ledger.Append(ctx, domain.Step{
		RunID: start.RunID, Kind: domain.StepRunStarted,
		Scope: start.Scope, AgentID: start.AgentID,
		VersionID: start.VersionID, OnBehalfOf: start.OnBehalfOf,
		Labels:  labels,
		Payload: mustJSON(domain.RunStartedPayload{Trigger: "webhook"}),
	}); err != nil {
		t.Fatalf("open run: %v", err)
	}

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	step, err := h.stepOf(t, domain.StepApprovalRequested)
	if err != nil {
		t.Fatal(err)
	}
	if !step.Labels.Has(domain.LabelUntrusted) ||
		!step.Labels.Has(domain.LabelArea(start.Scope)) {
		t.Fatalf("approval labels = %v, want the labels a person is deciding on", step.Labels)
	}
}

func TestAdvance_reserveFailureDoesNotRecordAToolCall(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	h.tools.reserveErr = errors.New("rate limited")

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err == nil {
		t.Fatal("Advance succeeded, want reserve failure")
	}
	got := h.kinds(t)
	if slices.Contains(got, domain.StepBudgetReserved) ||
		slices.Contains(got, domain.StepToolCalled) ||
		slices.Contains(got, domain.StepToolReturned) {
		t.Fatalf("ledger = %v, want no budget or tool steps after a reserve failure", got)
	}
	if len(h.tools.invocations) != 0 {
		t.Fatalf("tool invoked %d times after reserve failed", len(h.tools.invocations))
	}
}

func TestAdvance_writeTool_suspendsAwaitingApprovalWithoutInvoking(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.note", Args: []byte(`{"text":"hi"}`)})

	st, err := h.runner.Advance(ctx, h.start(t, generousBudget()))
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}

	if st.Phase != PhaseAwaitingApproval {
		t.Errorf("Phase = %v, want %v", st.Phase, PhaseAwaitingApproval)
	}
	if len(h.tools.invocations) != 0 {
		t.Errorf("tool was invoked %v while awaiting approval", h.tools.invocations)
	}
}

func TestAdvance_blockedTool_recordsDecisionAndCausesNoEffect(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.refund", Args: []byte(`{"amount":500}`)})

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	if len(h.tools.invocations) != 0 {
		t.Errorf("a blocked tool was invoked: %v", h.tools.invocations)
	}
	steps, _ := h.ledger.Read(ctx, "run-1", domain.FirstSeq)
	last := steps[len(steps)-1]
	if last.Kind != domain.StepGateDecided {
		t.Fatalf("last step = %s, want %s", last.Kind, domain.StepGateDecided)
	}
	// The block is in the trail with the rule that caused it, so the operator
	// sees which rule to change rather than "denied by policy" (PRD AU-10).
	if last.PolicyHash == "" {
		t.Error("gate decision recorded without a policy hash")
	}
}

func TestAdvance_unknownTool_blockedBeforeReachingTheCatalog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "payments.transfer", Args: []byte(`{}`)})

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if len(h.tools.invocations) != 0 {
		t.Errorf("a tool outside the pack was invoked: %v", h.tools.invocations)
	}
}

func TestAdvance_budgetTooSmallForTheCall_parksResumably(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{
		Tool:     "crm.lookup",
		Args:     []byte(`{}`),
		Estimate: domain.Consumption{Micros: 900_000},
	})

	st, err := h.runner.Advance(ctx, h.start(t, domain.Budget{Micros: 10_000, Steps: 50}))
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}

	if st.Phase != PhaseParked {
		t.Errorf("Phase = %v, want %v", st.Phase, PhaseParked)
	}
	if len(h.tools.invocations) != 0 {
		t.Errorf("tool invoked despite an exhausted budget: %v", h.tools.invocations)
	}
	// Parked, not finished: raising the ceiling must resume the same run.
	if st.Done {
		t.Error("Done = true for a parked run, which must stay resumable")
	}
}

// The most expensive bug this architecture can have. A worker dies after the
// tool call is recorded but before the result comes back; the run is picked up
// again and must not bill the customer twice (PRD DE-16, NF-02).
func TestResume_crashedAfterToolCall_doesNotInvokeTheToolAgain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	start := h.start(t, generousBudget())

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("first Advance: %v", err)
	}
	if len(h.tools.invocations) != 1 {
		t.Fatalf("invocations = %d after first pass, want 1", len(h.tools.invocations))
	}

	// A second worker picks the run up and replays the same proposal.
	h.planner.calls = 0
	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("resumed Advance: %v", err)
	}

	if len(h.tools.invocations) != 1 {
		t.Errorf("invocations = %d after resume, want 1 — the effect was duplicated",
			len(h.tools.invocations))
	}
}

func TestAdvance_confirmedSemanticDedupe_recordsDuplicateWithoutCallingTool(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	dedupeStore := &fakeDedupeStore{
		lookupFound: []bool{true},
		lookupRecords: []effectdedupe.Record{{
			State: effectdedupe.StateConfirmed, RunID: "run-old", Seq: 7,
		}},
	}
	enableDedupe(h, dedupeStore)

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if len(h.tools.invocations) != 0 {
		t.Fatalf("tool invoked despite confirmed semantic dedupe: %v", h.tools.invocations)
	}
	if len(dedupeStore.reserves) != 0 {
		t.Fatalf("Reserve called for a confirmed duplicate: %d", len(dedupeStore.reserves))
	}
	var decided domain.GateDecidedPayload
	if err := h.lastPayloadOf(t, domain.StepGateDecided, &decided); err != nil {
		t.Fatalf("gate payload: %v", err)
	}
	if decided.Verdict != domain.VerdictDuplicate || decided.Rule != gate.RuleIdempotency {
		t.Fatalf("decision = %s/%s, want duplicate/idempotency", decided.Verdict, decided.Rule)
	}
	if decided.Duplicate == nil || decided.Duplicate.RunID != "run-old" || decided.Duplicate.Seq != 7 {
		t.Fatalf("duplicate source = %+v, want run-old step 7", decided.Duplicate)
	}
	if slices.Contains(h.kinds(t), domain.StepToolCalled) {
		t.Fatalf("ledger = %v, want no tool call for a duplicate", h.kinds(t))
	}
}

func TestAdvance_confirmedSemanticDedupeWithoutSource_stillRecordsDuplicate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	dedupeStore := &fakeDedupeStore{
		lookupFound: []bool{false},
		reserveRecords: []effectdedupe.Record{{
			State: effectdedupe.StateConfirmed,
		}},
	}
	enableDedupe(h, dedupeStore)
	start := h.start(t, generousBudget())

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	var decided domain.GateDecidedPayload
	if err := h.lastPayloadOf(t, domain.StepGateDecided, &decided); err != nil {
		t.Fatalf("gate payload: %v", err)
	}
	if decided.Verdict != domain.VerdictDuplicate || decided.Rule != gate.RuleIdempotency {
		t.Fatalf("decision = %s/%s, want duplicate/idempotency", decided.Verdict, decided.Rule)
	}
	if decided.Duplicate != nil {
		t.Fatalf("duplicate source = %+v, want absent source for an unnamed duplicate", decided.Duplicate)
	}
	if slices.Contains(h.kinds(t), domain.StepToolCalled) {
		t.Fatalf("ledger = %v, want no tool call for a duplicate", h.kinds(t))
	}
}

func TestAdvance_confirmedSemanticDedupeSourceDoesNotDecorateEarlierBlocks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.refund", Args: []byte(`{"id":"42"}`)})
	dedupeStore := &fakeDedupeStore{
		lookupFound: []bool{true},
		lookupRecords: []effectdedupe.Record{{
			State: effectdedupe.StateConfirmed, RunID: "run-old", Seq: 7,
		}},
	}
	h.runner.deps.Catalog = dedupeCatalog{
		staticCatalog: staticCatalog{
			"crm.refund": domain.EffectFinancial,
		},
		dedupes: map[domain.ToolID]domain.ToolDedupe{
			"crm.refund": {WindowSeconds: 3600, ArgPaths: []string{"id"}},
		},
	}
	h.runner.deps.Dedupe = dedupeStore
	start := h.start(t, generousBudget())
	if _, err := h.ledger.Append(ctx, domain.Step{
		RunID: start.RunID, Kind: domain.StepRunStarted,
		Scope: start.Scope, AgentID: start.AgentID,
		VersionID: start.VersionID, OnBehalfOf: start.OnBehalfOf,
		Labels:  domain.NewLabels(domain.LabelUntrusted),
		Payload: mustJSON(domain.RunStartedPayload{Trigger: start.Trigger}),
	}); err != nil {
		t.Fatalf("start run: %v", err)
	}

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	var decided domain.GateDecidedPayload
	if err := h.lastPayloadOf(t, domain.StepGateDecided, &decided); err != nil {
		t.Fatalf("gate payload: %v", err)
	}
	if decided.Verdict != domain.VerdictBlock || decided.Rule != gate.RuleTaint {
		t.Fatalf("decision = %s/%s, want taint block", decided.Verdict, decided.Rule)
	}
	if decided.Duplicate != nil {
		t.Fatalf("duplicate source leaked onto a non-duplicate decision: %+v", decided.Duplicate)
	}
}

func TestAdvance_successfulSemanticDedupe_confirmsAfterToolReturn(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	dedupeStore := &fakeDedupeStore{lookupFound: []bool{false}}
	enableDedupe(h, dedupeStore)

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	called, err := h.stepOf(t, domain.StepToolCalled)
	if err != nil {
		t.Fatal(err)
	}
	if len(dedupeStore.confirms) != 1 {
		t.Fatalf("confirms = %d, want one", len(dedupeStore.confirms))
	}
	got := dedupeStore.confirms[0]
	if got.run != "run-1" || got.seq != called.Seq {
		t.Fatalf("confirmation = %+v, want run-1 seq %d", got, called.Seq)
	}
	if got.key.Scope != (domain.Scope{Company: "acme", Area: "cx"}) ||
		got.key.AgentID != "triage" || got.key.Tool != "crm.lookup" ||
		got.key.Fingerprint == "" {
		t.Fatalf("dedupe key missing platform-owned prefix: %+v", got.key)
	}
	if len(dedupeStore.releases) != 0 {
		t.Fatalf("release called after a successful tool return: %d", len(dedupeStore.releases))
	}
}

func TestAdvance_failedSemanticDedupeCall_releasesReservation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	h.tools.failed = true
	dedupeStore := &fakeDedupeStore{lookupFound: []bool{false}}
	enableDedupe(h, dedupeStore)

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if len(dedupeStore.releases) != 1 {
		t.Fatalf("releases = %d, want one after failed tool return", len(dedupeStore.releases))
	}
	if len(dedupeStore.confirms) != 0 {
		t.Fatalf("confirmed a failed tool call: %d", len(dedupeStore.confirms))
	}
}

func TestAdvance_confirmFailureAfterEffectLeavesDoesNotMarkItDone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	dedupeStore := &fakeDedupeStore{
		lookupFound: []bool{false},
		confirmErr:  errors.New("database unavailable"),
	}
	enableDedupe(h, dedupeStore)

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err == nil {
		t.Fatal("Advance succeeded, want confirmation failure")
	}
	got := h.kinds(t)
	if !slices.Contains(got, domain.StepToolReturned) ||
		!slices.Contains(got, domain.StepBudgetReconciled) {
		t.Fatalf("ledger = %v, want tool return and budget reconciliation before confirm failure", got)
	}
	if len(dedupeStore.confirms) != 1 {
		t.Fatalf("confirms = %d, want one attempted confirmation", len(dedupeStore.confirms))
	}
}

func TestAdvance_pendingSemanticDedupeWaitsForConfirmationInsteadOfParking(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	dedupeStore := &fakeDedupeStore{
		lookupFound: []bool{false, true},
		lookupRecords: []effectdedupe.Record{
			{},
			{State: effectdedupe.StateConfirmed, RunID: "run-old", Seq: 9},
		},
		reserveRecords: []effectdedupe.Record{{
			State: effectdedupe.StatePending, RunID: "run-old",
		}},
	}
	enableDedupe(h, dedupeStore)

	st, err := h.runner.Advance(ctx, h.start(t, generousBudget()))
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if st.Phase == PhaseParked {
		t.Fatal("pending dedupe parked the run; parked means a person must act")
	}
	if len(h.tools.invocations) != 0 {
		t.Fatalf("tool invoked while another run confirmed the same effect: %v", h.tools.invocations)
	}
	var decided domain.GateDecidedPayload
	if err := h.lastPayloadOf(t, domain.StepGateDecided, &decided); err != nil {
		t.Fatalf("gate payload: %v", err)
	}
	if decided.Verdict != domain.VerdictDuplicate {
		t.Fatalf("Verdict = %s, want duplicate", decided.Verdict)
	}
	if decided.Duplicate == nil || decided.Duplicate.RunID != "run-old" || decided.Duplicate.Seq != 9 {
		t.Fatalf("duplicate source = %+v, want run-old step 9", decided.Duplicate)
	}
}

func TestAdvance_pendingSemanticDedupeTimeoutIsRetryableSupervisionState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	dedupeStore := &fakeDedupeStore{
		lookupFound: []bool{false},
		reserveRecords: []effectdedupe.Record{{
			State: effectdedupe.StatePending, RunID: "run-old",
		}},
	}
	enableDedupe(h, dedupeStore)
	h.runner.deps.DedupePendingWait = time.Millisecond
	h.runner.deps.DedupePendingPoll = time.Millisecond

	_, err := h.runner.Advance(ctx, h.start(t, generousBudget()))
	if err == nil {
		t.Fatal("Advance succeeded, want the run to retry under supervision")
	}
	var summarized interface {
		Summary() domain.FailureSummary
	}
	if !errors.As(err, &summarized) {
		t.Fatalf("error %v does not expose a stable failure summary", err)
	}
	failure := summarized.Summary()
	if failure.Code != CodeDedupeInFlight || !failure.Retryable {
		t.Fatalf("failure = %+v, want retryable %s", failure, CodeDedupeInFlight)
	}
	if len(h.tools.invocations) != 0 {
		t.Fatalf("tool invoked while another run owned the same effect: %v", h.tools.invocations)
	}
	if slices.Contains(h.kinds(t), domain.StepParked) {
		t.Fatalf("pending dedupe parked inside the engine; steps: %v", h.kinds(t))
	}
}

// A worker that dies between recording the call and receiving the result
// leaves an orphan: the effect may or may not have landed, and nothing in the
// process knows which. The loop must close it out honestly instead of guessing.
func TestAdvance_resumedWithOrphanedToolCall_closesItAndReleasesBudget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{}`)})
	start := h.start(t, generousBudget())

	// Hand-build the ledger of a run that crashed mid-call.
	for _, s := range []domain.Step{
		{Kind: domain.StepRunStarted},
		{Kind: domain.StepBudgetReserved,
			Payload: mustJSON(domain.BudgetReservedPayload{Micros: 30_000})},
		{Kind: domain.StepToolCalled, IdemKey: "already-done",
			Payload: mustJSON(domain.ToolCalledPayload{Tool: "crm.lookup"})},
	} {
		s.RunID, s.Scope = start.RunID, start.Scope
		s.At = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
		if _, err := h.ledger.Append(ctx, s); err != nil {
			t.Fatalf("seed Append(%s): %v", s.Kind, err)
		}
	}

	st, err := h.runner.Advance(ctx, start)
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}

	// The orphan is closed and the run is runnable again, but the loop did not
	// re-invoke: it has no idea whether the first call took effect.
	if st.Phase != PhaseRunning {
		t.Errorf("Phase = %v, want %v", st.Phase, PhaseRunning)
	}
	if len(h.tools.invocations) != 0 {
		t.Errorf("tool invoked while recovering an orphaned call: %v", h.tools.invocations)
	}

	steps, _ := h.ledger.Read(ctx, "run-1", domain.FirstSeq)
	state, err := Fold(steps)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	if state.Reserved.Micros != 0 {
		t.Errorf("Reserved.Micros = %d, want 0 — the reservation leaked", state.Reserved.Micros)
	}
	// The recorded idempotency key survives the recovery, so a re-plan that
	// proposes the same call still cannot repeat the effect.
	if !state.AlreadyExecuted("already-done") {
		t.Error("recovery lost the idempotency key of the orphaned call")
	}
}

func TestAdvance_plannerReportsDone_finishesRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t) // no proposals: the planner reports done immediately

	st, err := h.runner.Advance(ctx, h.start(t, generousBudget()))
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}

	if st.Phase != PhaseFinished || !st.Done {
		t.Errorf("Phase = %v, Done = %v, want finished/true", st.Phase, st.Done)
	}

	var finished domain.RunFinishedPayload
	if err := h.payloadOf(t, domain.StepRunFinished, &finished); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if finished.Reason != domain.RunFinishedByFinishTool {
		t.Errorf("reason = %q, want %q", finished.Reason, domain.RunFinishedByFinishTool)
	}
}

func TestAdvance_finishStoresNamedContextArtifacts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{
		Done:      true,
		Outcome:   "completed",
		Artifacts: map[string]string{"triage_summary": "root cause: cache saturation"},
	})
	start := h.start(t, generousBudget())
	if _, err := h.ledger.Append(ctx, domain.Step{
		RunID: start.RunID, Kind: domain.StepRunStarted,
		Scope: start.Scope, AgentID: start.AgentID,
		VersionID: start.VersionID, OnBehalfOf: start.OnBehalfOf,
		Labels:  domain.NewLabels(domain.LabelUntrusted),
		Payload: mustJSON(domain.RunStartedPayload{Trigger: "event"}),
	}); err != nil {
		t.Fatalf("open run: %v", err)
	}

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var finished domain.RunFinishedPayload
	if err := h.payloadOf(t, domain.StepRunFinished, &finished); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if len(finished.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v, want one", finished.Artifacts)
	}
	artifact := finished.Artifacts[0]
	if artifact.Name != "triage_summary" || artifact.Kind != "text" ||
		artifact.SourceRun != start.RunID || artifact.SourceAgent != start.AgentID ||
		artifact.Digest != digest([]byte("root cause: cache saturation")) ||
		!artifact.Labels.Has(domain.LabelUntrusted) {
		t.Fatalf("artifact = %+v", artifact)
	}
	got, err := h.content.Get(ctx, artifact.Ref)
	if err != nil {
		t.Fatalf("artifact content: %v", err)
	}
	if string(got) != "root cause: cache saturation" {
		t.Fatalf("artifact body = %q", got)
	}
}

func TestAdvance_plannerReturnsTextWithoutAction_parksInsteadOfFinishing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Outcome: "Vou continuar analisando."})

	st, err := h.runner.Advance(ctx, h.start(t, generousBudget()))
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}

	if st.Phase != PhaseParked || st.Done {
		t.Fatalf("Phase = %v, Done = %v, want parked/not done", st.Phase, st.Done)
	}
	if len(h.tools.invocations) != 0 {
		t.Fatalf("empty proposal reached the tool layer: %v", h.tools.invocations)
	}
	var parked domain.ParkedPayload
	if err := h.payloadOf(t, domain.StepParked, &parked); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if parked.Reason != "no_finish_action" {
		t.Errorf("reason = %q, want no_finish_action", parked.Reason)
	}
}

func TestAdvance_toolFails_recordsFailureAndReleasesTheReservation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{}`)})
	h.tools.err = errors.New("connection refused")

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	steps, _ := h.ledger.Read(ctx, "run-1", domain.FirstSeq)
	state, err := Fold(steps)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	// A reservation left outstanding after a failure would leak budget for the
	// rest of the run and eventually park it for no reason.
	if state.Reserved.Micros != 0 {
		t.Errorf("Reserved.Micros = %d after a failed call, want 0", state.Reserved.Micros)
	}
}

func TestAdvance_finishedRun_isNotAdvancedFurther(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t)
	start := h.start(t, generousBudget())

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("first Advance: %v", err)
	}
	before := len(h.kinds(t))

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("second Advance: %v", err)
	}

	if after := len(h.kinds(t)); after != before {
		t.Errorf("a finished run gained %d steps on a second Advance", after-before)
	}
}

func TestAdvance_planKeepsProposingABlockedCall_parksInsteadOfLooping(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// A block is fed back so the model can choose differently. When it does
	// not, every further turn is a paid model call that cannot succeed: the
	// pack, the taint and the policy are all fixed for the run's version.
	refused := Proposal{Tool: "payments.transfer", Args: []byte(`{}`)}
	h := newHarness(t, slices.Repeat([]Proposal{refused}, maxConsecutiveBlocks+2)...)
	start := h.start(t, generousBudget())

	var last Status
	for range maxConsecutiveBlocks + 1 {
		var err error
		if last, err = h.runner.Advance(ctx, start); err != nil {
			t.Fatalf("Advance: %v", err)
		}
	}

	if last.Phase != PhaseParked {
		t.Fatalf("Phase = %v after %d refused proposals, want parked",
			last.Phase, maxConsecutiveBlocks+1)
	}
	if len(h.tools.invocations) != 0 {
		t.Errorf("a refused tool was invoked: %v", h.tools.invocations)
	}
}

func TestAdvance_planKeepsProposingADuplicateCall_parksInsteadOfLooping(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// A duplicate is not a Gate refusal: no effect leaves the platform, and
	// the planner sees a skip rather than a denial. If it keeps proposing the
	// same skipped effect anyway, the run still has to stop spending model
	// turns on a path that cannot make progress.
	repeated := Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)}
	h := newHarness(t, slices.Repeat([]Proposal{repeated}, maxConsecutiveBlocks+2)...)
	start := h.start(t, generousBudget())

	var last Status
	for range maxConsecutiveBlocks + 2 {
		var err error
		if last, err = h.runner.Advance(ctx, start); err != nil {
			t.Fatalf("Advance: %v", err)
		}
	}

	if last.Phase != PhaseParked {
		t.Fatalf("Phase = %v after repeated duplicate proposals, want parked; steps: %v",
			last.Phase, h.kinds(t))
	}
	if len(h.tools.invocations) != 1 {
		t.Errorf("tool invocations = %d, want only the original call", len(h.tools.invocations))
	}
}

func TestAdvance_blockThenProgress_doesNotCountTowardsParking(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// One refusal followed by a call the Gate allows is the feedback loop
	// working. Counting it towards the no-progress ceiling would park agents
	// that recovered exactly as intended.
	refused := Proposal{Tool: "payments.transfer", Args: []byte(`{}`)}
	allowed := Proposal{Tool: "crm.lookup", Args: []byte(`{}`)}

	h := newHarness(t, append(
		slices.Repeat([]Proposal{refused}, maxConsecutiveBlocks-1),
		allowed, refused,
	)...)
	start := h.start(t, generousBudget())

	var last Status
	for range maxConsecutiveBlocks + 1 {
		var err error
		if last, err = h.runner.Advance(ctx, start); err != nil {
			t.Fatalf("Advance: %v", err)
		}
	}

	if last.Phase == PhaseParked {
		t.Errorf("the run parked despite an allowed call between refusals; steps: %v", h.kinds(t))
	}
}

// The proposed arguments are the thing being decided. Two readers need them
// and neither can get them from the ledger: the model, replaying its own call
// on the next turn, and the human being asked to approve it. Holding only a
// digest makes the first replay an empty call and the second a blind decision.

func TestAdvance_toolCalled_storesArgumentsSoTheCallReplaysWithThem(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	args := []byte(`{"id":"42"}`)
	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: args})

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	steps, err := h.ledger.Read(ctx, "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	turns, err := BuildTranscript(ctx, h.content, steps)
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}

	i := slices.IndexFunc(turns, func(turn Turn) bool { return turn.Kind == TurnToolUse })
	if i < 0 {
		t.Fatal("the transcript has no tool call to replay")
	}
	if string(turns[i].Args) != string(args) {
		t.Errorf("replayed args = %q, want %q", turns[i].Args, args)
	}
}

func TestAdvance_toolLevelFailureReplaysAsFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	h.tools.failed = true
	h.tools.errorCode = "tool_error"
	h.tools.body = []byte("customer not found")

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var returned domain.ToolReturnedPayload
	if err := h.payloadOf(t, domain.StepToolReturned, &returned); err != nil {
		t.Fatalf("no tool_returned step: %v", err)
	}
	if !returned.Failed || returned.ErrorCode != "tool_error" {
		t.Fatalf("returned failure = (%v, %q), want tool_error", returned.Failed, returned.ErrorCode)
	}

	steps, err := h.ledger.Read(ctx, "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	turns, err := BuildTranscript(ctx, h.content, steps)
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}
	i := slices.IndexFunc(turns, func(turn Turn) bool { return turn.Kind == TurnToolResult })
	if i < 0 {
		t.Fatal("the transcript has no tool result")
	}
	if !turns[i].Failed || string(turns[i].Content) != "customer not found" {
		t.Errorf("tool result = (failed %v, content %q), want failed customer not found",
			turns[i].Failed, turns[i].Content)
	}
}

func TestAdvance_invokeFailureKeepsTheDiagnosticReference(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	h.tools.err = errors.New("transport refused the call")
	h.tools.failed = true
	h.tools.errorCode = "invoke_error"
	h.tools.body = []byte("the tool failed: invoke_error\ntransport refused the call")

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var returned domain.ToolReturnedPayload
	if err := h.payloadOf(t, domain.StepToolReturned, &returned); err != nil {
		t.Fatalf("no tool_returned step: %v", err)
	}
	if !returned.Failed || returned.ErrorCode != "invoke_error" {
		t.Fatalf("returned failure = (%v, %q), want invoke_error", returned.Failed, returned.ErrorCode)
	}
	if returned.ResultRef == "" {
		t.Fatal("invoke failure lost the diagnostic content reference")
	}

	steps, err := h.ledger.Read(ctx, "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	turns, err := BuildTranscript(ctx, h.content, steps)
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}
	i := slices.IndexFunc(turns, func(turn Turn) bool { return turn.Kind == TurnToolResult })
	if i < 0 {
		t.Fatal("the transcript has no tool result")
	}
	if got := string(turns[i].Content); got != "the tool failed: invoke_error\ntransport refused the call" {
		t.Errorf("tool result content = %q", got)
	}
}

func TestAdvance_approvalRequested_recordsWhatTheApproverIsDeciding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	args := []byte(`{"text":"refunded per ticket 4471"}`)
	h := newHarness(t, Proposal{
		Tool:     "crm.note",
		Args:     args,
		Estimate: domain.Consumption{Micros: 12_000, Tokens: 900},
	})

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var asked domain.ApprovalRequestedPayload
	if err := h.payloadOf(t, domain.StepApprovalRequested, &asked); err != nil {
		t.Fatalf("no approval_requested step: %v", err)
	}

	if asked.Effect != domain.EffectWrite {
		t.Errorf("effect = %q, want write — the approver has to know it writes", asked.Effect)
	}
	if asked.Estimate.Micros != 12_000 {
		t.Errorf("estimate = %d micros, want 12000", asked.Estimate.Micros)
	}
	if asked.ArgsDigest == "" {
		t.Error("no args digest: the decision cannot be tied to the arguments it was made about")
	}

	stored, err := h.content.Get(ctx, asked.ArgsRef)
	if err != nil {
		t.Fatalf("the approver cannot see the proposed arguments: %v", err)
	}
	if string(stored) != string(args) {
		t.Errorf("stored args = %q, want %q", stored, args)
	}
}

// What a rule did has to reach the trail, or the policy screen cannot count a
// single thing and a monitored rule looks like a bug.

func TestAdvance_policyDecision_recordsWhichRuleAndWhatWasOnlyWatching(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t, Proposal{Tool: "crm.note", Args: []byte(`{"text":"hi"}`)})
	h.runner.deps.Gate = gate.New().WithPolicies(gate.Policies{
		Hash: "pol_test",
		Set: []domain.Policy{
			{
				Code: "POL-114", Resource: "crm.*", Reach: domain.ReachInstallation,
				Effect: domain.PolicyDeny, Mode: domain.PolicyEnforce, Enabled: true,
				Reason: "escritas em crm passam por revisão",
			},
			{
				Code: "POL-900", Resource: "*", Reach: domain.ReachInstallation,
				Effect: domain.PolicyEscalate, Mode: domain.PolicyMonitor, Enabled: true,
			},
		},
	})

	if _, err := h.runner.Advance(ctx, h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var decided domain.GateDecidedPayload
	if err := h.payloadOf(t, domain.StepGateDecided, &decided); err != nil {
		t.Fatalf("no gate_decided step: %v", err)
	}

	if decided.PolicyCode != "POL-114" {
		t.Errorf("policy_code = %q, want the rule that fired", decided.PolicyCode)
	}
	if len(decided.Monitored) != 1 || decided.Monitored[0].Code != "POL-900" {
		t.Errorf("monitored = %+v, want the watching rule recorded", decided.Monitored)
	}
}

func TestAdvance_afterAnApproval_executesTheCallThatWasApproved(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// The whole point of the approval queue. Granting one and then asking the
	// model what to do next means the approval decided nothing: the Gate sees
	// a fresh proposal, requires approval again, and the run loops until it
	// parks — which is what it did.
	h := newHarness(t,
		Proposal{Tool: "crm.note", Args: []byte(`{"text":"hi"}`)},
		// A second proposal, so a run that wrongly replans reaches for it and
		// the test can tell the two apart.
		Proposal{Tool: "crm.lookup", Args: []byte(`{"email":"x@y.z"}`)},
	)
	start := h.start(t, generousBudget())

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	h.approve(t, true)

	st, err := h.runner.Advance(ctx, start)
	if err != nil {
		t.Fatalf("Advance after approval: %v", err)
	}

	if len(h.tools.invocations) != 1 {
		t.Fatalf("invocations = %v, want the approved call to have run", h.tools.invocations)
	}
	if h.tools.invocations[0] != "crm.note" {
		t.Errorf("invoked %q, want the tool the person approved", h.tools.invocations[0])
	}
	if st.Phase == PhaseAwaitingApproval {
		t.Error("asked for approval again on a call somebody already approved")
	}
}

func TestAdvance_afterARejection_doesNotExecute(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	h := newHarness(t,
		Proposal{Tool: "crm.note", Args: []byte(`{"text":"hi"}`)},
		Proposal{Tool: "crm.lookup", Args: []byte(`{"email":"x@y.z"}`)},
	)
	start := h.start(t, generousBudget())

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	h.approve(t, false)

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance after rejection: %v", err)
	}

	for _, called := range h.tools.invocations {
		if called == "crm.note" {
			t.Fatalf("invoked %q after a person refused it", called)
		}
	}
}

// approve records a person's decision the way the API does.
func (h *harness) approve(t *testing.T, granted bool) {
	t.Helper()

	head, err := h.ledger.Head(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if _, err := h.ledger.Append(context.Background(), domain.Step{
		RunID: "run-1", Kind: domain.StepApprovalDecided,
		Scope:   domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: "v3", At: head.At,
		Payload: mustJSON(domain.ApprovalDecidedPayload{Approved: granted, By: "ana"}),
	}); err != nil {
		t.Fatalf("append approval: %v", err)
	}
}

func TestAdvance_toolCallCeilingReached_blocksTheNextCall(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// A ceiling of one call has to mean one call. The engine never told the
	// Gate that the request it was ruling on was itself a call, so every
	// tool-call ceiling let one more through than it said. The Gate's own test
	// fixture filled the field in, which is how the gap survived: the fake was
	// more careful than the thing it stood for.
	h := newHarness(t,
		Proposal{Tool: "crm.lookup", Args: []byte(`{"a":1}`)},
		Proposal{Tool: "crm.lookup", Args: []byte(`{"a":2}`)},
	)
	start := h.start(t, domain.Budget{Micros: 1_000_000, ToolCalls: 1, Steps: 40})

	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	if len(h.tools.invocations) != 1 {
		t.Errorf("invocations = %v, want one call against a ceiling of one",
			h.tools.invocations)
	}

	var decided domain.GateDecidedPayload
	if err := h.lastPayloadOf(t, domain.StepGateDecided, &decided); err != nil {
		t.Fatalf("read final gate decision: %v", err)
	}
	if decided.Breached != "tool calls" {
		t.Fatalf("breached = %q, want the dimension that stopped the run", decided.Breached)
	}
	if decided.Budget == nil || decided.Committed == nil ||
		decided.Estimate == nil || decided.Projected == nil {
		t.Fatalf("budget evidence = budget %+v committed %+v estimate %+v projected %+v",
			decided.Budget, decided.Committed, decided.Estimate, decided.Projected)
	}
	if decided.Budget.ToolCalls != 1 || decided.Committed.ToolCalls != 1 ||
		decided.Estimate.ToolCalls != 1 || decided.Projected.ToolCalls != 2 {
		t.Fatalf("budget evidence = budget %+v committed %+v estimate %+v projected %+v",
			decided.Budget, decided.Committed, decided.Estimate, decided.Projected)
	}
}

func TestAdvance_pastTheWallClockCeiling_parksResumably(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// The fourth ceiling. It was declared in the specification, accepted by
	// the API and reported on the agent screen, and no run ever hit it.
	h := newHarnessOn(t,
		&tickingClock{t: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)},
		Proposal{Tool: "crm.lookup", Args: []byte(`{"a":1}`)},
		Proposal{Tool: "crm.lookup", Args: []byte(`{"a":2}`)},
	)
	start := h.start(t, domain.Budget{Micros: 1_000_000, WallClockMS: 1, Steps: 40})

	// The clock moves a second per step, so by the second turn the run is far
	// past a ceiling of one millisecond.
	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	if got := h.kinds(t); got[len(got)-1] != domain.StepParked {
		t.Errorf("last step = %q, want the run parked at its wall-clock ceiling; steps: %v",
			got[len(got)-1], got)
	}
}
