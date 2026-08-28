package memory

import (
	"slices"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Copying a value out, and the two small normalisations everything shares.
//
// The stores hand back copies rather than what they hold: a caller that mutated
// a returned assertion would be editing the store through a value it was only
// shown.

func cloneAssertion(a domain.MemoryAssertion) domain.MemoryAssertion {
	a.Labels = a.Labels.Clone()
	a.Evidence = cloneEvidence(a.Evidence)
	return a
}

func cloneSuggestion(s domain.MemorySuggestion) domain.MemorySuggestion {
	s.Labels = s.Labels.Clone()
	s.Evidence = cloneEvidence(s.Evidence)
	return s
}

// cloneEvidence copies the labels inside each citation too. Cloning only the
// slice of records leaves every Labels sharing its backing array, and the
// in-memory store hands out what it holds — so a reader mutating a returned
// citation would be editing the stored taint in place.
func cloneEvidence(in []domain.MemoryEvidence) []domain.MemoryEvidence {
	out := slices.Clone(in)
	for i := range out {
		out[i].Labels = out[i].Labels.Clone()
	}
	return out
}

/*
boundedEvidence folds repeated citations and holds the record count to the cap.

Folded by MemoryEvidence.Key rather than by the whole record, because the labels
are resolved from the ledger at the moment somebody asks: the same step comes
back clean today and tainted once the run that produced it gained a label, and
those are one citation read twice. Keeping both would spend the budget on a
duplicate of itself; keeping only the first would discard the later, fuller
reading. So the labels are unioned, which is also the direction that never loses
a taint.
*/
func boundedEvidence(in []domain.MemoryEvidence) []domain.MemoryEvidence {
	out := make([]domain.MemoryEvidence, 0, min(len(in), domain.MaxMemoryEvidence))
	at := map[string]int{}
	for _, ev := range in {
		if i, seen := at[ev.Key()]; seen {
			out[i].Labels = out[i].Labels.Union(ev.Labels)
			continue
		}
		if len(out) == domain.MaxMemoryEvidence {
			continue
		}
		at[ev.Key()] = len(out)
		out = append(out, ev)
	}
	return out
}

func clean(v string) string { return strings.TrimSpace(v) }

func nowOrWall(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
