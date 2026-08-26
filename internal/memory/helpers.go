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

func first(in []domain.MemoryAssertion, n int) []domain.MemoryAssertion {
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

func clean(v string) string { return strings.TrimSpace(v) }

func nowOrWall(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
