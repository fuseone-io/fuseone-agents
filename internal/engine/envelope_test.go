package engine

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

// A run advances through its steps by calling their tools, and never goes
// back. Nothing judges whether a step is "done": the proposal itself moves the
// run forward, which needs no model and no condition written in prose.

func TestEnvelopeOf_noSteps_isThePackItself(t *testing.T) {
	t.Parallel()

	start := Start{Pack: gate.NewPack("a", "b")}

	if got := envelopeOf(start, nil).Tools(); len(got) != 2 {
		t.Errorf("got %v, want the whole pack", got)
	}
}

func TestEnvelopeOf_beforeAnythingIsCalled_reachesEveryStep(t *testing.T) {
	t.Parallel()

	start := Start{Steps: []Envelope{{Name: "um", Reaches: []domain.ToolID{"lookup"}}, {Name: "dois"}, {Name: "tres", Reaches: []domain.ToolID{"reply"}}}}

	// Forward is open; it is going back that is refused. Forbidding the
	// author's second-favourite ordering would describe their first draft
	// rather than their process.
	got := envelopeOf(start, nil).Tools()
	if len(got) != 2 {
		t.Errorf("got %v, want lookup and reply", got)
	}
}

func TestEnvelopeOf_afterALaterStepIsUsed_theEarlierOneIsClosed(t *testing.T) {
	t.Parallel()

	start := Start{Steps: []Envelope{{Name: "um", Reaches: []domain.ToolID{"lookup"}}, {Name: "dois"}, {Name: "tres", Reaches: []domain.ToolID{"reply"}}}}

	// Having replied, the run cannot look anything up again. That is the
	// guarantee steps exist to make, and it holds without anybody deciding
	// that a step is over.
	got := envelopeOf(start, []domain.ToolID{"reply"}).Tools()
	if len(got) != 1 || got[0] != "reply" {
		t.Errorf("got %v, want reply alone", got)
	}
}

func TestEnvelopeOf_aToolInNoStep_isReachableNowhere(t *testing.T) {
	t.Parallel()

	start := Start{
		Pack:  gate.NewPack("lookup", "purge"),
		Steps: []Envelope{{Name: "um", Reaches: []domain.ToolID{"lookup"}}},
	}

	// Declaring steps narrows the pack rather than restating it: a tool left
	// out of every step was granted and then never placed, and the safe
	// reading of that is that nobody meant it to run.
	if envelopeOf(start, nil).Allows("purge") {
		t.Error("a tool in no step should be unreachable")
	}
}

func TestStepAt_aPlatformToolAnchoredInALateStep_doesNotAdvanceTheRun(t *testing.T) {
	t.Parallel()

	// The outage that was not one (#152): an author listed a platform tool in
	// a late step, the model consulted memory as its opening move, and the run
	// jumped past every earlier step — their tools gone with no park and no
	// note, because the narrowing was "legitimate" forward movement. A
	// platform tool is an ambient capability; using one is not evidence the
	// run did that step's work.
	start := Start{Steps: []Envelope{
		{Name: "um", Reaches: []domain.ToolID{"lookup"}},
		{Name: "dois", Reaches: []domain.ToolID{"reply"}},
		{Name: "tres", Reaches: []domain.ToolID{domain.ToolMemoryFind}},
	}}

	called := []domain.ToolID{domain.ToolMemoryFind}
	if at := StepAt(start, called); at != 0 {
		t.Errorf("StepAt = %d, want 0: a memory lookup is not step work", at)
	}
	if !envelopeOf(start, called).Allows("lookup") {
		t.Error("the first step closed because the run consulted memory")
	}
}

func TestClosedTools_namesWhatForwardMovementPutBehind(t *testing.T) {
	t.Parallel()

	// "shared" appears again in a later step, so moving forward never closes
	// it; "lookup" and "probe" exist only behind the run, and they are what
	// the trail must name — a narrowed envelope nothing narrates reads
	// exactly like a tool outage (#152).
	start := Start{Steps: []Envelope{
		{Name: "um", Reaches: []domain.ToolID{"lookup"}},
		{Name: "dois", Reaches: []domain.ToolID{"probe", "shared"}},
		{Name: "tres", Reaches: []domain.ToolID{"reply", "shared"}},
	}}

	if got := closedTools(start, State{}); got != nil {
		t.Errorf("closedTools before any call = %v, want none", got)
	}

	got := closedTools(start, State{Called: []domain.ToolID{"reply"}})
	if len(got) != 2 || got[0] != "lookup" || got[1] != "probe" {
		t.Errorf("closedTools = %v, want lookup and probe, in step order", got)
	}
}

func TestClosedTools_aPlatformToolTheTurnStillOffers_isNotBehind(t *testing.T) {
	t.Parallel()

	// An author anchored memory.find in the first step; memory learning
	// re-offers it on every turn regardless. After the run advances, the
	// tool is still in the prompt — a payload calling it closed would be
	// the trail contradicting the surface the model actually saw (#153
	// review). The judge must be the turn's effective surface, not the
	// bare step union.
	start := Start{
		MemoryLearning: domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview},
		Steps: []Envelope{
			{Name: "um", Reaches: []domain.ToolID{"lookup", domain.ToolMemoryFind}},
			{Name: "dois", Reaches: []domain.ToolID{"reply"}},
		},
	}

	got := closedTools(start, State{Called: []domain.ToolID{"reply"}})
	if len(got) != 1 || got[0] != "lookup" {
		t.Errorf("closedTools = %v, want lookup alone: memory.find is still offered", got)
	}
}

func TestAdvance_afterForwardMovement_thePlannedStepNamesTheClosedTools(t *testing.T) {
	t.Parallel()

	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	start := h.start(t, generousBudget())
	start.Steps = []Envelope{
		{Name: "prepare", Reaches: []domain.ToolID{"crm.note"}},
		{Name: "investigate", Reaches: []domain.ToolID{"crm.lookup"}},
	}

	ctx := context.Background()
	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance 1: %v", err)
	}
	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance 2: %v", err)
	}

	var p domain.PlannedPayload
	if err := h.lastPayloadOf(t, domain.StepPlanned, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Node != "investigate" {
		t.Errorf("node = %q, want investigate", p.Node)
	}
	if len(p.ClosedTools) != 1 || p.ClosedTools[0] != "crm.note" {
		t.Errorf("closedTools = %v, want the step the run moved past", p.ClosedTools)
	}
}

func TestAdvance_aSpentFailureAfterForwardMovement_stillRecordsTheClosedTools(t *testing.T) {
	t.Parallel()

	// The billed-failure path builds its own planned payload. Leaving
	// ClosedTools out of that one constructor keeps every other test green
	// and the trail mute exactly where the money went (#153 review).
	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"42"}`)})
	start := h.start(t, generousBudget())
	start.Steps = []Envelope{
		{Name: "prepare", Reaches: []domain.ToolID{"crm.note"}},
		{Name: "investigate", Reaches: []domain.ToolID{"crm.lookup"}},
	}

	ctx := context.Background()
	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance 1: %v", err)
	}
	h.runner.deps.Planner = &spendingPlanner{err: &SpentPlanError{
		Cost:     domain.Cost{Micros: 1200, InputTokens: 300},
		Provider: "anthropic",
		Model:    "claude-opus-5",
		Err:      context.DeadlineExceeded,
	}}
	if _, err := h.runner.Advance(ctx, start); err == nil {
		t.Fatal("Advance swallowed the planner failure")
	}

	var p domain.PlannedPayload
	if err := h.lastPayloadOf(t, domain.StepPlanned, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if len(p.ClosedTools) != 1 || p.ClosedTools[0] != "crm.note" {
		t.Errorf("closedTools = %v, want the billed prompt's closed step", p.ClosedTools)
	}
}

func TestEnvelopeForState_memorySuggestRequiresLearningPolicy(t *testing.T) {
	t.Parallel()

	start := Start{
		Pack: gate.NewPack("crm.lookup", domain.ToolMemorySuggest),
	}
	got := envelopeForState(start, State{})
	if got.Allows(domain.ToolMemorySuggest) {
		t.Fatal("memory suggest was offered from the pack while memory learning was off")
	}
	if !got.Allows("crm.lookup") {
		t.Fatal("ordinary pack tool was removed while filtering memory suggest")
	}

	start.MemoryLearning = domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview}
	got = envelopeForState(start, State{})
	if !got.Allows(domain.ToolMemorySuggest) {
		t.Fatal("memory suggest was not offered when memory learning was enabled")
	}
}

func TestEnvelopeForState_memoryLearningOffersMemoryLookup(t *testing.T) {
	t.Parallel()

	start := Start{
		Pack:           gate.NewPack("crm.lookup"),
		MemoryLearning: domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview},
	}
	got := envelopeForState(start, State{})
	if !got.Allows(domain.ToolMemoryFind) {
		t.Fatal("memory find was not offered when memory learning was enabled")
	}
	if !got.Allows(domain.ToolMemorySuggest) {
		t.Fatal("memory suggest was not offered when memory learning was enabled")
	}
	if !got.Allows("crm.lookup") {
		t.Fatal("ordinary pack tool was removed while adding memory tools")
	}
}

func TestSpendAt_pricesTheStepBeingEnteredNotTheOneJustFinished(t *testing.T) {
	t.Parallel()

	// The lever the PRD asks for: an economical model where the agent
	// classifies, a strong one where it decides (FO-10, FO-11).
	start := Start{Steps: []Envelope{
		{Name: "Triar", Reaches: []domain.ToolID{"crm.lookup"}, Model: "claude-haiku-4-5", Effort: "low"},
		{Name: "Decidir", Reaches: []domain.ToolID{"crm.reply"}, Model: "claude-opus-5", Effort: "high"},
	}}

	model, effort := SpendAt(start, nil)
	if model != "claude-haiku-4-5" || effort != "low" {
		t.Errorf("triage spends %q/%q, want the economical model", model, effort)
	}

	// The run has looked the customer up. What happens next is the decision,
	// and pricing it at the step behind would hand the cheap model to exactly
	// the reasoning the expensive one was configured for.
	model, effort = SpendAt(start, []domain.ToolID{"crm.lookup"})
	if model != "claude-opus-5" || effort != "high" {
		t.Errorf("the decision spends %q/%q, want the strong model", model, effort)
	}
}

func TestSpendAt_aPlatformToolCall_doesNotAdvanceTheSpend(t *testing.T) {
	t.Parallel()

	// The engine's own initial memory lookup fills Called before any step
	// work happens. Pricing that as "entering the second step" hands the
	// strong model's rate to reasoning still sitting in the first — the same
	// invariant StepAt guards, on the money side (#153 review).
	start := Start{Steps: []Envelope{
		{Name: "Triar", Reaches: []domain.ToolID{"crm.lookup"}, Model: "claude-haiku-4-5", Effort: "low"},
		{Name: "Decidir", Reaches: []domain.ToolID{"crm.reply"}, Model: "claude-opus-5", Effort: "high"},
	}}

	model, effort := SpendAt(start, []domain.ToolID{domain.ToolMemoryFind})
	if model != "claude-haiku-4-5" || effort != "low" {
		t.Errorf("spends %q/%q, want the first step's: a memory lookup is not step work", model, effort)
	}
}

func TestSpendAt_pastTheLastStep_staysOnIt(t *testing.T) {
	t.Parallel()

	// A run that has used the last step's tool is finishing, not entering
	// something that does not exist.
	start := Start{Steps: []Envelope{
		{Name: "Responder", Reaches: []domain.ToolID{"crm.reply"}, Model: "claude-opus-5"},
	}}
	model, _ := SpendAt(start, []domain.ToolID{"crm.reply"})
	if model != "claude-opus-5" {
		t.Errorf("spends %q", model)
	}
}

func TestSpendAt_noStepsDeclared_isTheAgentsThroughout(t *testing.T) {
	t.Parallel()

	model, effort := SpendAt(Start{}, []domain.ToolID{"crm.lookup"})
	if model != "" || effort != "" {
		t.Errorf("spends %q/%q, want the agent's", model, effort)
	}
}
