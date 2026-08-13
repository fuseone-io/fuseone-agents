package e2e_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Erasing a person's data leaves the audit record standing (PRD AU-04).

This is the entire reason bulky and personal payloads live behind a reference
and a digest instead of inside the step. It was argued for in the design and
asserted nowhere: the erasure tests checked the tombstone and that one
subject's erasure spares another's, never that the trail still verifies
afterwards.

The consequence of getting it wrong is not subtle — an installation that
honoured an erasure request would find its whole audit record unverifiable —
but the cause would be: storing one payload inline is enough to do it.
*/
func TestErase_theChainStillVerifiesAndTheTrailStillReads(t *testing.T) {
	eachLedger(t, "erasing content does not touch the hash chain", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, readThenAnswer)
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.lookup", Effect: domain.EffectRead,
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.open(t, "run-erase-1")
		if state := p.settle(t, "run-erase-1"); state.Phase != engine.PhaseFinished {
			t.Fatalf("phase = %v, want the run to have finished", state.Phase)
		}

		before := p.steps(t, "run-erase-1")
		ref := resultRef(t, before)

		erased, err := p.content.Erase(t.Context(), "run-erase-1", "subject request")
		if err != nil {
			t.Fatalf("Erase: %v", err)
		}
		if erased == 0 {
			t.Fatal("erased nothing; the run filed no content to erase")
		}

		// The claim, in three parts. The chain still verifies, every step is
		// still there, and the digest that proves what the erased payload was
		// is still on the trail.
		if err := store.Verify(t.Context(), "run-erase-1"); err != nil {
			t.Errorf("Verify after erasure: %v", err)
		}
		after := p.steps(t, "run-erase-1")
		if len(after) != len(before) {
			t.Errorf("steps = %d after erasure, want the %d from before",
				len(after), len(before))
		}
		for i := range after {
			if !bytes.Equal(after[i].Hash, before[i].Hash) {
				t.Errorf("step %d changed hash; erasure reached the ledger", after[i].Seq)
			}
		}

		// And the payload really is gone, reported as erased rather than as
		// a reference that was always wrong.
		if _, err := p.content.Get(t.Context(), ref); !errors.Is(err, ledger.ErrErased) {
			t.Errorf("Get after erasure = %v, want it reported as erased", err)
		}
	})
}

// resultRef is where the run filed what a tool returned.
func resultRef(t *testing.T, steps []domain.Step) string {
	t.Helper()

	for _, step := range steps {
		if step.Kind != domain.StepToolReturned {
			continue
		}
		var p domain.ToolReturnedPayload
		if err := json.Unmarshal(step.Payload, &p); err != nil {
			t.Fatalf("decode %d: %v", step.Seq, err)
		}
		if p.ResultRef != "" {
			return p.ResultRef
		}
	}
	t.Fatal("no tool result was filed; there is nothing to erase")
	return ""
}
