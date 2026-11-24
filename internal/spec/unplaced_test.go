package spec_test

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

/*
A run of an agent with steps is offered only what some step reaches: a tool in
the pack that no step reaches is never offered, wherever the run is. The
definition still publishes, and a run then reports the tool as not available.
Unplaced is that list, so the workbench can say it before a run finds out.
*/

func TestUnplaced_aToolNoStepReaches_isListed(t *testing.T) {
	t.Parallel()
	s := mustParse(t, withSteps)

	if got := s.Unplaced(); !slices.Equal(got, []domain.ToolID{"kb.search"}) {
		t.Errorf("Unplaced = %v, want [kb.search]", got)
	}
}

func TestUnplaced_noSteps_isNothing(t *testing.T) {
	t.Parallel()
	s := spec.Spec{Tools: []domain.ToolID{"crm.lookup", "kb.search"}}

	if got := s.Unplaced(); len(got) != 0 {
		t.Errorf("Unplaced = %v, want nothing: with no steps a run keeps its whole pack", got)
	}
}

func TestUnplaced_aPlatformTool_isNeverListed(t *testing.T) {
	t.Parallel()
	s := spec.Spec{
		Tools: []domain.ToolID{domain.ToolMemoryFind, "crm.lookup"},
		Steps: []spec.Step{{Name: "Look up", Reaches: []domain.ToolID{"crm.lookup"}}},
	}

	if got := s.Unplaced(); len(got) != 0 {
		t.Errorf("Unplaced = %v, want nothing: the platform offers its own tools whatever the step", got)
	}
}

func mustParse(t *testing.T, doc string) spec.Spec {
	t.Helper()
	s, err := spec.Parse("test", []byte(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return s
}
