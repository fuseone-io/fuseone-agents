package simulate

import (
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

// CorpusChange is how the corpus differs now from the version a battery ran.
type CorpusChange struct {
	// NotChecked is what nobody has checked as it now is: added since, or the same id
	// with another input or other expectations. Neither broken nor held, and it does
	// not stop a start by itself. Running the battery again checks it.
	NotChecked []string
	// ErasedSince is what was added or changed and has already lost its input. The
	// battery never mentions it, and running it again would not bring it back.
	ErasedSince []string
}

/*
Since compares the version a battery ran with the corpus as it is.

The version and not the battery's runs. A case of the version that never opened a run is
broken, which the battery already says. And one that kept its id and changed what it
expects has a run, which answered another question. A case deleted says nothing: stopping
a check is the author's act, and asks nothing of anybody.
*/
func Since(pinned, working []domain.RegressionCase) CorpusChange {
	at := make(map[string]domain.RegressionCase, len(pinned))
	for _, c := range pinned {
		at[c.ID] = c
	}
	var out CorpusChange
	for _, c := range working {
		before, had := at[c.ID]
		switch {
		case had && unchanged(before, c):
		case c.Erased:
			out.ErasedSince = append(out.ErasedSince, c.ID)
		default:
			out.NotChecked = append(out.NotChecked, c.ID)
		}
	}
	return out
}

// unchanged compares what a case asks. Once erased, both sides read as no expectations,
// so an erasure is not a change.
func unchanged(before, now domain.RegressionCase) bool {
	return before.InputRef == now.InputRef && slices.Equal(before.Expectations, now.Expectations)
}
