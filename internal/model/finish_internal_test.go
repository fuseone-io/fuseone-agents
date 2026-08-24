package model

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// A finished planning call keeps the pair it was made against.
//
// finishProposal used to rebuild the proposal from a list of fields, so the
// provider and model were dropped the moment they existed — and a finished run
// would carry cost with nothing saying which model spent it. Building from the
// call's own proposal makes the next field survive too.
func TestFinishProposal_keepsTheProviderAndModel(t *testing.T) {
	t.Parallel()

	got := finishProposal([]byte(`{
		"summary":"done",
		"artifacts":{"triage_summary":" root cause ","empty":" "}
	}`), engine.Proposal{
		Provider: "anthropic",
		Model:    "claude-haiku-4-5",
		Cost:     domain.Cost{Micros: 900},
	})

	if got.Provider != "anthropic" || got.Model != "claude-haiku-4-5" {
		t.Fatalf("finish dropped the pair: %q/%q", got.Provider, got.Model)
	}
	if !got.Done || got.Outcome != "done" {
		t.Errorf("finish did not record the ending: %+v", got)
	}
	if got.Artifacts["triage_summary"] != "root cause" || len(got.Artifacts) != 1 {
		t.Errorf("artifacts = %+v, want the named non-empty artifact only", got.Artifacts)
	}
}
