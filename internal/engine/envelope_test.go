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
