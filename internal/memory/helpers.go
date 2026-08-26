package memory

import (
	"slices"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

func prepareAssertion(
	a domain.MemoryAssertion, by domain.UserID, now time.Time,
) (domain.MemoryAssertion, error) {
	a.Kind = clean(a.Kind)
	a.Subject = clean(a.Subject)
	a.Signature = clean(a.Signature)
	a.Claim = clean(a.Claim)
	if a.Status == "" {
		a.Status = domain.MemoryActive
	}
	if err := a.Validate(); err != nil {
		return domain.MemoryAssertion{}, err
	}
	a.ID = domain.MemoryAssertionID(a)
	a.CreatedBy, a.UpdatedBy = by, by
	a.CreatedAt, a.UpdatedAt = now.UTC(), now.UTC()
	a.Labels = a.Labels.Union(domain.ScopeLabels(a.Scope))
	a.Evidence = slices.Clone(a.Evidence)
	return a, nil
}

func prepareSuggestion(
	s domain.MemorySuggestion, by domain.UserID, now time.Time,
) (domain.MemorySuggestion, error) {
	s.Kind = clean(s.Kind)
	s.Subject = clean(s.Subject)
	s.Signature = clean(s.Signature)
	s.Claim = clean(s.Claim)
	if s.Status == "" {
		s.Status = domain.MemorySuggestionPending
	}
	if s.Observations <= 0 {
		s.Observations = 1
	}
	s.AssertionID = domain.MemoryAssertionID(domain.MemoryAssertion{
		Scope: s.Scope, AgentID: s.AgentID, Kind: s.Kind,
		Subject: s.Subject, Signature: s.Signature, Claim: s.Claim,
		Evidence: s.Evidence, Observations: s.Observations,
		Status: domain.MemoryActive,
	})
	s.ID = domain.MemorySuggestionID(s)
	s.CreatedBy, s.UpdatedBy = by, by
	s.CreatedAt, s.UpdatedAt = now.UTC(), now.UTC()
	s.Labels = s.Labels.Union(domain.ScopeLabels(s.Scope))
	s.Evidence = boundedEvidence(s.Evidence)
	if err := s.Validate(); err != nil {
		return domain.MemorySuggestion{}, err
	}
	return s, nil
}

// A suggestion is duplicate if it matches active memory the same run could
// read: first the agent namespace, then the shared namespace in the same scope.
func activeAssertionIDsForSuggestion(s domain.MemorySuggestion) []string {
	ids := []string{s.AssertionID}
	if s.AgentID == "" {
		return ids
	}
	shared := domain.MemoryAssertionID(domain.MemoryAssertion{
		Scope: s.Scope, Kind: s.Kind, Subject: s.Subject,
		Signature: s.Signature,
	})
	if shared != s.AssertionID {
		ids = append(ids, shared)
	}
	return ids
}

func memoryFindMatches(a domain.MemoryAssertion, q domain.MemoryQuery) bool {
	if a.Scope != q.Scope || !(a.AgentID == "" || a.AgentID == q.AgentID) {
		return false
	}
	if a.Status != domain.MemoryActive || expired(a, q.Now) {
		return false
	}
	return matchesField(a.Kind, q.Kind) && matchesField(a.Subject, q.Subject) &&
		matchesField(a.Signature, q.Signature) && containsSearch(a, q.Search)
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

func suggestionContainsSearch(s domain.MemorySuggestion, q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(s.Subject), q) ||
		strings.Contains(strings.ToLower(s.Signature), q) ||
		strings.Contains(strings.ToLower(s.Claim), q)
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

func containsSearch(a domain.MemoryAssertion, q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(a.Subject), q) ||
		strings.Contains(strings.ToLower(a.Signature), q) ||
		strings.Contains(strings.ToLower(a.Claim), q)
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

func cloneAssertion(a domain.MemoryAssertion) domain.MemoryAssertion {
	a.Labels = a.Labels.Clone()
	a.Evidence = slices.Clone(a.Evidence)
	return a
}

func cloneSuggestion(s domain.MemorySuggestion) domain.MemorySuggestion {
	s.Labels = s.Labels.Clone()
	s.Evidence = slices.Clone(s.Evidence)
	return s
}

func boundedEvidence(in []domain.MemoryEvidence) []domain.MemoryEvidence {
	out := make([]domain.MemoryEvidence, 0, min(len(in), domain.MaxMemoryEvidence))
	seen := map[domain.MemoryEvidence]bool{}
	for _, ev := range in {
		if seen[ev] {
			continue
		}
		seen[ev] = true
		out = append(out, ev)
		if len(out) == domain.MaxMemoryEvidence {
			break
		}
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
