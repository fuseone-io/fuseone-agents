package engine

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

// The planning step has to name the model the platform requested.
//
// Cost, tokens and price provenance are already recorded per planning call,
// and none of them says which model they belong to — so a FinOps aggregate
// built on them would attribute spend by guessing, and "the rate was
// configured" would be a claim about a model nobody named. A step that chooses
// its own model is the case that makes the guess wrong.
func TestPlanned_recordsTheRequestedModel(t *testing.T) {
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
		t.Fatalf("recorded %q/%q, want the pair the platform requested",
			p.Provider, p.Model)
	}
}

func TestPlanned_recordsTheRequestedAndServedModelsSeparately(t *testing.T) {
	t.Parallel()

	h := newHarness(t, Proposal{
		Done: true, Provider: "litellm", Model: "opus-5-5", ServedModel: "opus-5",
	})
	if _, err := h.runner.Advance(context.Background(), h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var p domain.PlannedPayload
	if err := h.payloadOf(t, domain.StepPlanned, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Model != "opus-5-5" || p.ServedModel != "opus-5" {
		t.Fatalf("models = requested %q / served %q", p.Model, p.ServedModel)
	}
}
