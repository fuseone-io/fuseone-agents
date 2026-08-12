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
		Steps: [][]domain.ToolID{
			{"crm.lookup"},
			{"kb.search"},
			{"crm.reply"},
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
