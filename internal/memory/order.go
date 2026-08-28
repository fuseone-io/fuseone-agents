package memory

import (
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

// How a result set is ordered and how much of it comes back.
//
// Ranking, not matching. What a row must match is decided in filter.go and in
// the query; this is only about which of the rows that already matched comes
// first, and where the list is cut.

func sortAssertions(out []domain.MemoryAssertion) {
	slices.SortFunc(out, func(a, b domain.MemoryAssertion) int {
		if a.Confirmed != b.Confirmed {
			return int(b.Confirmed - a.Confirmed)
		}
		if a.Observations != b.Observations {
			return int(b.Observations - a.Observations)
		}
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func sortFindAssertions(out []domain.MemoryAssertion, search string) {
	terms := searchTerms(search)
	if len(terms) == 0 {
		sortAssertions(out)
		return
	}
	slices.SortFunc(out, func(a, b domain.MemoryAssertion) int {
		if scoreA, scoreB := searchScore(a, terms), searchScore(b, terms); scoreA != scoreB {
			return scoreB - scoreA
		}
		if a.Confirmed != b.Confirmed {
			return int(b.Confirmed - a.Confirmed)
		}
		if a.Observations != b.Observations {
			return int(b.Observations - a.Observations)
		}
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func sortByUpdated(out []domain.MemoryAssertion) {
	slices.SortFunc(out, func(a, b domain.MemoryAssertion) int {
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func sortSuggestionsByUpdated(out []domain.MemorySuggestion) {
	slices.SortFunc(out, func(a, b domain.MemorySuggestion) int {
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func first(in []domain.MemoryAssertion, n int) []domain.MemoryAssertion {
	if len(in) <= n {
		return in
	}
	return in[:n]
}

func firstSuggestions(in []domain.MemorySuggestion, n int) []domain.MemorySuggestion {
	if len(in) <= n {
		return in
	}
	return in[:n]
}
