package engine

import (
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
