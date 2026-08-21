package simulate_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/simulate"
)

// liveTools is a tool layer that acts. Nothing in a simulation may reach it,
// and this is here to prove nothing does.
type liveTools struct{ calls int }

func (l *liveTools) Reserve(context.Context, engine.Call) error { return nil }

func (l *liveTools) Invoke(context.Context, engine.Call) (engine.ToolResult, error) {
	l.calls++
	return engine.ToolResult{}, nil
}

func TestDeps_dropsTheToolLayerItWasHanded(t *testing.T) {
	t.Parallel()

	live := &liveTools{}
	got := simulate.Deps(engine.Deps{Tools: live, Clock: engine.SystemClock{}})

	if _, err := got.Tools.Invoke(t.Context(), engine.Call{Tool: "crm.refund"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	// The property the whole feature rests on: whatever the caller was
	// holding, a simulated run has no path to a real system.
	if live.calls != 0 {
		t.Fatalf("the real tool layer was invoked %d times", live.calls)
	}
	// And the rest of the loop is untouched, or a simulation would be
	// deciding by different rules than the thing it stands in for.
	if got.Clock == nil {
		t.Error("Deps dropped more than the tool layer")
	}
}

func TestDryTools_answersWithNothingRatherThanFailing(t *testing.T) {
	t.Parallel()

	got, err := simulate.DryTools{}.Invoke(t.Context(), engine.Call{Tool: "crm.note"})
	if err != nil {
		// A refusal would end the run at the first write, and the case would
		// report "stopped at step 4" when nobody let it try.
		t.Fatalf("Invoke: %v", err)
	}
	// Never a fabricated reference, and never a label: nothing was read, so
	// nothing can be tainted by it, and the next decision must not be a
	// decision about data the run never saw.
	if got.ResultRef != "" || len(got.Labels) != 0 || got.Failed {
		t.Errorf("result = %+v, want nothing", got)
	}
}
