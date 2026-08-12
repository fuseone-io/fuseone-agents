package simulate_test

import (
	"testing"

	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/simulate"
)

// A dry run is a real run with one thing missing: the tool call. Everything
// else happens — the Gate decides, the ledger records, the budget reserves —
// because the point of a simulation is to show where the policy would have
// stopped it, and a simulation that skipped the Gate would answer a question
// nobody asked.

func TestDryTools_neverInvokeAnything(t *testing.T) {
	t.Parallel()

	dry := simulate.NewDryTools()
	got, err := dry.Invoke(t.Context(), engine.Call{
		RunID: "sim_1", Seq: 3, Tool: "crm.reply", Args: []byte(`{"texto":"olá"}`),
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	// It answers rather than failing. A refusal would end the run at the
	// first write, and the case would report "stopped at step 4" when what
	// happened is that nobody let it try.
	if got.Failed {
		t.Errorf("a dry call must not read as a failure: %+v", got)
	}
	if len(dry.Calls()) != 1 || dry.Calls()[0].Tool != "crm.reply" {
		t.Errorf("got %+v", dry.Calls())
	}
}

func TestDryTools_aReadIsAnsweredWithNothingRatherThanInvented(t *testing.T) {
	t.Parallel()

	dry := simulate.NewDryTools()
	got, _ := dry.Invoke(t.Context(), engine.Call{Tool: "crm.lookup"})

	// No reference, so the transcript carries no fabricated customer. A made
	// up answer would make the rest of the case a story about data that does
	// not exist — and the author would be reviewing it as though it were real.
	if got.ResultRef != "" {
		t.Errorf("a dry read invented an answer: %+v", got)
	}
}

func TestDryTools_recordOnlyWhatTheyWereGiven(t *testing.T) {
	t.Parallel()

	dry := simulate.NewDryTools()
	if _, err := dry.Invoke(t.Context(), engine.Call{Tool: "crm.reply", Seq: 4}); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	// The effect is not here, and should not be: the Gate decided it before
	// the call and wrote it to the ledger. The report is a fold of that
	// ledger, like every other projection in this product — building a second
	// account of the same run here would be a second thing to keep in step.
	if got := dry.Calls()[0]; got.Tool != "crm.reply" || got.Seq != 4 {
		t.Errorf("got %+v", got)
	}
}
