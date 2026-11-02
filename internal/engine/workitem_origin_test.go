package engine

import (
	"reflect"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
Rolling upgrade for run_started.work_item (#204, slice 3).

A worker one release behind folds starts a newer opener sealed: the field is
unknown to it, and an unknown field on run_started is ignored exactly as the
2b stamp on tool_returned was. And this release's fold takes no decision from
the inheritance — attribution only — so the state is the state of the same
start without it.
*/
func TestFold_aStartCarryingAWorkItem_foldsAsTheSameStartWithout(t *testing.T) {
	t.Parallel()
	start := func(payload string) []domain.Step {
		return []domain.Step{{RunID: "run-1", Seq: domain.FirstSeq, Kind: domain.StepRunStarted,
			AgentID: "reviewer", At: time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC), Payload: []byte(payload)}}
	}
	without, err := Fold(start(`{"trigger":"event","chain":{"root":"run-0","hop":1}}`))
	if err != nil {
		t.Fatalf("Fold an old-shape start: %v", err)
	}
	with, err := Fold(start(`{"trigger":"event","chain":{"root":"run-0","hop":1},` +
		`"work_item":{"ref":"github:o/r#1","source":"event","from_run":"run-0"},` +
		`"a_field_from_a_newer_release":{"x":1}}`))
	if err != nil {
		t.Fatalf("Fold a start with a work item = %v; an older or newer worker would fail the run", err)
	}
	if !reflect.DeepEqual(with, without) {
		t.Fatalf("the inheritance changed the fold:\n with    %+v\n without %+v", with, without)
	}
}
