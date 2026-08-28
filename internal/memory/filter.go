package memory

import (
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Whether one row answers one question.
//
// The in-memory store's half of what search_postgres.go builds in SQL. They are
// two implementations of one contract, which is why the suites run against
// both: a fake that matched more loosely than the query would certify
// behaviour production does not have.

func memoryFindMatches(a domain.MemoryAssertion, q domain.MemoryQuery) bool {
	if a.Scope != q.Scope || !(a.AgentID == "" || a.AgentID == q.AgentID) {
		return false
	}
	if a.Status != domain.MemoryActive || expired(a, q.Now) {
		return false
	}
	return matchesField(a.Kind, q.Kind) && matchesField(a.Subject, q.Subject) &&
		matchesField(a.Signature, q.Signature) && findSearchMatches(a, q.Search)
}

func suggestionMatches(s domain.MemorySuggestion, f SuggestionFilter) bool {
	if !containsScope(f.Scopes, s.Scope) {
		return false
	}
	if f.AgentID != "" && !(s.AgentID == "" || s.AgentID == f.AgentID) {
		return false
	}
	if f.Status.Valid() && s.Status != f.Status {
		return false
	}
	return suggestionContainsSearch(s, f.Search)
}

func listMatches(a domain.MemoryAssertion, f Filter) bool {
	if !containsScope(f.Scopes, a.Scope) {
		return false
	}
	if f.AgentID != "" && !(a.AgentID == "" || a.AgentID == f.AgentID) {
		return false
	}
	if f.Status.Valid() && effectiveStatus(a, f.Now).Status != f.Status {
		return false
	}
	return containsSearch(a, f.Search)
}

func effectiveStatus(a domain.MemoryAssertion, now time.Time) domain.MemoryAssertion {
	if a.Status == domain.MemoryActive && expired(a, now) {
		a.Status = domain.MemoryExpired
	}
	return a
}

func expired(a domain.MemoryAssertion, now time.Time) bool {
	return a.ExpiresAt != nil && !a.ExpiresAt.After(now.UTC())
}

func containsScope(scopes []domain.Scope, scope domain.Scope) bool {
	for _, s := range scopes {
		if s.Contains(scope) {
			return true
		}
	}
	return false
}

func matchesField(got, want string) bool {
	return want == "" || got == strings.TrimSpace(want)
}
