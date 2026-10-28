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

/*
The headline is the one line a channel shows for the run; the full answer
hangs in its thread (#165). The model declares it through the finish tool —
nothing is inferred from the response, on the stopped_by precedent — and the
mechanical guard keeps only the first line: a multi-line headline posted at a
channel's top level is the wallpaper the field exists to end.
*/
func TestFinishProposal_headlineKeepsItsFirstLineOnly(t *testing.T) {
	t.Parallel()

	got := finishProposal([]byte(`{
		"summary":"the full report",
		"headline":"  2 achados · pior: CRÍTICO \nsegunda linha que não sobe  "
	}`), engine.Proposal{})

	if got.Headline != "2 achados · pior: CRÍTICO" {
		t.Errorf("Headline = %q, want the trimmed first line", got.Headline)
	}
	if without := finishProposal([]byte(`{"summary":"done"}`), engine.Proposal{}); without.Headline != "" {
		t.Errorf("an absent headline invented %q", without.Headline)
	}
}

/*
A run cannot publish an artifact called final_answer.

The name is already taken: it is what a citation calls the run's closing
answer, and the resolver answers it from OutcomeRef before it ever looks at
what the run published (citationIn). So an artifact under that name is bytes
nothing can cite — and worse than unreachable, because the console offering it
would record a memory pointing at the closing answer while saying it points
here.

Dropped rather than refused. The model asked for a name it cannot have, which
is not a reason to fail a run that has already produced its answer.
*/
func TestFinishProposal_artifactNamedLikeTheClosingAnswer_isNotPublished(t *testing.T) {
	t.Parallel()

	got := finishProposal([]byte(`{
		"summary":"done",
		"artifacts":{"final_answer":"something else","report":"kept"}
	}`), engine.Proposal{})

	if _, taken := got.Artifacts[domain.ArtifactFinalAnswer]; taken {
		t.Errorf("artifacts = %+v, want the reserved name dropped", got.Artifacts)
	}
	if got.Artifacts["report"] != "kept" {
		t.Errorf("artifacts = %+v, want the rest published", got.Artifacts)
	}
	if got.Outcome != "done" {
		t.Errorf("outcome = %q, want the run to finish anyway", got.Outcome)
	}
}

// The same name in any spelling a person or a model would write it. The
// citation's own comparison is exact, so this is about what reaches the ledger
// rather than about matching loosely later.
func TestFinishProposal_reservedNameIsMatchedWhateverTheSpelling(t *testing.T) {
	t.Parallel()

	got := finishProposal([]byte(`{
		"summary":"done",
		"artifacts":{" Final_Answer ":"something else"}
	}`), engine.Proposal{})

	if len(got.Artifacts) != 0 {
		t.Errorf("artifacts = %+v, want the reserved name dropped", got.Artifacts)
	}
}
