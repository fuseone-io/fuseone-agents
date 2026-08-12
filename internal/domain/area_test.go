package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

// An area is typed by a person and then referenced by a budget, a policy and
// every agent filed under it. Two spellings of one area are two areas that
// never meet, and nothing in the product would report the mistake: the ceiling
// simply never applies.

func TestNormalizeAreaID_spellingsOfOneArea_foldToOneID(t *testing.T) {
	t.Parallel()

	for _, typed := range []string{"cx", "CX", " cx ", "Cx"} {
		got, err := domain.NormalizeAreaID(typed)
		if err != nil {
			t.Fatalf("NormalizeAreaID(%q): %v", typed, err)
		}
		if got != domain.AreaID("cx") {
			t.Errorf("NormalizeAreaID(%q) = %q, want cx", typed, got)
		}
	}
}

func TestNormalizeAreaID_wordsSeparatedBySpace_joinWithAHyphen(t *testing.T) {
	t.Parallel()

	got, err := domain.NormalizeAreaID("Risco de Crédito")
	if err != nil {
		t.Fatalf("NormalizeAreaID: %v", err)
	}
	if got != domain.AreaID("risco-de-credito") {
		t.Errorf("got %q, want risco-de-credito", got)
	}
}

func TestNormalizeAreaID_charactersThatCannotSurviveAURL_rejected(t *testing.T) {
	t.Parallel()

	// The id reaches a path segment and a webhook route. One that has to be
	// escaped to be addressed is one somebody will address wrongly.
	for _, typed := range []string{"", "   ", "cx/prod", "cx?a=1", "..", "a#b"} {
		if got, err := domain.NormalizeAreaID(typed); err == nil {
			t.Errorf("NormalizeAreaID(%q) = %q, want an error", typed, got)
		}
	}
}
