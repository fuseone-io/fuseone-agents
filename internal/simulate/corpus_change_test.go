package simulate

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func correction(id, input, value string) domain.RegressionCase {
	return domain.RegressionCase{
		ID: id, InputRef: input,
		Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: value}},
	}
}

func erasedCorrection(id, input string) domain.RegressionCase {
	return domain.RegressionCase{ID: id, InputRef: input, Erased: true}
}

/*
What the corpus is now, against the version a battery ran.

Not against the runs: a case of the version that never opened one is broken, which the
battery already says, and a case that kept its id and changed what it expects has a run
that answered another question.
*/
func TestSince_comparesTheVersionWithTheCorpusAsItIs(t *testing.T) {
	t.Parallel()
	same := correction("igual", "in-1", "a")
	cases := []struct {
		name            string
		pinned, working []domain.RegressionCase
		notChecked      []string
		erasedSince     []string
	}{
		{name: "nothing moved", pinned: []domain.RegressionCase{same}, working: []domain.RegressionCase{same}},
		{
			name:       "a case added",
			pinned:     []domain.RegressionCase{same},
			working:    []domain.RegressionCase{same, correction("novo", "in-2", "a")},
			notChecked: []string{"novo"},
		},
		{
			name:       "the same id expecting something else",
			pinned:     []domain.RegressionCase{same},
			working:    []domain.RegressionCase{correction("igual", "in-1", "b")},
			notChecked: []string{"igual"},
		},
		{
			name:       "the same id on another input",
			pinned:     []domain.RegressionCase{same},
			working:    []domain.RegressionCase{correction("igual", "in-9", "a")},
			notChecked: []string{"igual"},
		},
		// The report never had it, so nothing else says this coverage is gone. Running
		// the battery again would not bring it back.
		{
			name:        "a case added and already erased",
			pinned:      []domain.RegressionCase{same},
			working:     []domain.RegressionCase{same, erasedCorrection("sumiu", "in-3")},
			erasedSince: []string{"sumiu"},
		},
		{
			name:        "a changed case erased since",
			pinned:      []domain.RegressionCase{same},
			working:     []domain.RegressionCase{erasedCorrection("igual", "in-9")},
			erasedSince: []string{"igual"},
		},
		// The version reads it as erased too, and the battery already calls it a gap.
		{
			name:    "a case of the version erased since",
			pinned:  []domain.RegressionCase{erasedCorrection("igual", "in-1")},
			working: []domain.RegressionCase{erasedCorrection("igual", "in-1")},
		},
		// Stopping a check is the author's act, and nobody has to run anything for it.
		{name: "a case deleted", pinned: []domain.RegressionCase{same}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Since(tc.pinned, tc.working)
			if !slices.Equal(got.NotChecked, tc.notChecked) {
				t.Errorf("not checked = %v, want %v", got.NotChecked, tc.notChecked)
			}
			if !slices.Equal(got.ErasedSince, tc.erasedSince) {
				t.Errorf("erased since = %v, want %v", got.ErasedSince, tc.erasedSince)
			}
		})
	}
}
