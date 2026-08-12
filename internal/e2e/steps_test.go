package e2e_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
)

// The guarantee steps exist to make: a run that has replied cannot look the
// customer up again. It holds without anything judging whether a stage is
// over — the proposal itself moved the run forward.

func TestStart_afterALaterStep_theEarlierToolsAreOutOfReach(t *testing.T) {
	t.Parallel()

	start := engine.Start{
		Pack: gate.NewPack("crm.lookup", "kb.search", "crm.reply"),
		Steps: []engine.Envelope{
			{Name: "Identificar", Reaches: []domain.ToolID{"crm.lookup"}},
			{Name: "Pesquisar", Reaches: []domain.ToolID{"kb.search"}},
			{Name: "Responder", Reaches: []domain.ToolID{"crm.reply"}},
		},
	}

	before := engine.EnvelopeFor(start, nil)
	after := engine.EnvelopeFor(start, []domain.ToolID{"crm.reply"})

	if !before.Allows("crm.lookup") {
		t.Error("a fresh run should reach the first step")
	}
	if after.Allows("crm.lookup") || after.Allows("kb.search") {
		t.Error("a run that replied must not go back")
	}
	if !after.Allows("crm.reply") {
		t.Error("the step it is in stays reachable")
	}
}

func TestPlanned_recordsTheStepTheRunWasIn(t *testing.T) {
	t.Parallel()

	start := engine.Start{
		Steps: []engine.Envelope{
			{Name: "Identificar", Reaches: []domain.ToolID{"crm.lookup"}},
			{Name: "Responder", Reaches: []domain.ToolID{"crm.reply"}},
		},
	}

	// A correction is anchored to the step it went wrong at, so the step has
	// to be in the record rather than reconstructed from it later.
	if got := engine.StepNameAt(start, nil); got != "Identificar" {
		t.Errorf("fresh run: got %q", got)
	}
	if got := engine.StepNameAt(start, []domain.ToolID{"crm.reply"}); got != "Responder" {
		t.Errorf("after replying: got %q", got)
	}
}
