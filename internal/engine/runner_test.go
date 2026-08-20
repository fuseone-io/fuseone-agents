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
	content     ContentStore
	body        []byte
	err         error
	failed      bool
	errorCode   string
}

func (c *countingTools) Invoke(ctx context.Context, call Call) (ToolResult, error) {
	c.invocations = append(c.invocations, call.Tool)
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
	steps, err := h.ledger.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	i := slices.IndexFunc(steps, func(s domain.Step) bool { return s.Kind == kind })
	if i < 0 {
		return fmt.Errorf("no %s step in %v", kind, h.kinds(t))
	}
	return json.Unmarshal(steps[i].Payload, into)
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
			"crm.lookup": domain.EffectRead,
			"crm.note":   domain.EffectWrite,
			"crm.refund": domain.EffectFinancial,
		},
		Clock: clock,
	})
	return h
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
