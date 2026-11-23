package blocks_test

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/authoring/blocks"
)

/*
An excerpt that occurs twice is refused even when the two occurrences overlap:
in "aaa", "aa" starts at two places, which a count of non-overlapping matches
reports as one, and the edit would change whichever the replace found first.
*/
func TestEdit_anExcerptOccurringTwice_isRefusedEvenOverlapping(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ text, old string }{
		"overlapping": {"Escale aaa ao plantão.", "aa"},
		"apart":       {"Leia. Responda. Leia.", "Leia."},
	} {
		doc := []blocks.Block{{ID: "b1", Kind: blocks.HowToAct, Label: "Como agir", Text: tc.text}}
		if _, err := blocks.Edit(doc, "b1", tc.old, "x"); !errors.Is(err, blocks.ErrExcerpt) {
			t.Errorf("%s: Edit = %v, want ErrExcerpt", name, err)
		}
	}
}

// An excerpt found once is replaced there, and nowhere else.
func TestEdit_anExcerptFoundOnce_isReplacedInPlace(t *testing.T) {
	t.Parallel()
	doc := []blocks.Block{{ID: "b1", Kind: blocks.HowToAct, Label: "Como agir", Text: "Escale aab ao plantão."}}
	got, err := blocks.Edit(doc, "b1", "aab", "o caso")
	if err != nil || got[0].Text != "Escale o caso ao plantão." {
		t.Fatalf("Edit = %q, %v; want the excerpt replaced in place", got[0].Text, err)
	}
}
