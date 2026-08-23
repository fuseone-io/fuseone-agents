package engine

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

// The planning step has to name the model it actually used.
//
// Cost, tokens and price provenance are already recorded per planning call,
// and none of them says which model they belong to — so a FinOps aggregate
// built on them would attribute spend by guessing, and "the rate was
// configured" would be a claim about a model nobody named. A step that chooses
// its own model is the case that makes the guess wrong.
func TestPlanned_recordsTheModelTheCallUsed(t *testing.T) {
	t.Parallel()

	h := newHarness(t, Proposal{
		Done:     true,
		Outcome:  "done",
		Provider: "anthropic",
		Model:    "claude-haiku-4-5",
	})
	if _, err := h.runner.Advance(context.Background(), h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var p domain.PlannedPayload
	if err := h.payloadOf(t, domain.StepPlanned, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Provider != "anthropic" || p.Model != "claude-haiku-4-5" {
		t.Fatalf("recorded %q/%q, want the pair the call was made against",
			p.Provider, p.Model)
	}
}
