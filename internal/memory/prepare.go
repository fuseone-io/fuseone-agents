package memory

import (
	"fmt"
	"slices"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// What a write becomes before anything looks at it.
//
// Cleaning, validating, stamping and deriving the identity — the small amount
// of work that has to happen identically whichever store is about to hold it,
// and which is why it does not live in either of them.

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
		return domain.MemoryAssertion{}, fmt.Errorf("%w: %v", ErrInvalid, err)
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
		return domain.MemorySuggestion{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return s, nil
}

/*
identitiesForSuggestion is where an equivalent memory could already be: the
agent's own namespace first, then the shared one every agent in the scope reads.

Identities rather than assertion ids. The id hashes the strings as typed, so a
memory somebody wrote as "Grafana Datasource" was invisible to an agent that
proposed "grafana  datasource" — and the queue filled with proposals for a fact
the platform already knew, each of which a person had to read and dismiss. The
id travels along because a row written before the canonical key answers to
nothing else.
*/
func identitiesForSuggestion(s domain.MemorySuggestion) []domain.MemoryAssertion {
	own := domain.MemoryAssertion{
		Scope: s.Scope, AgentID: s.AgentID, Kind: s.Kind,
		Subject: s.Subject, Signature: s.Signature,
	}
	own.ID = domain.MemoryAssertionID(own)
	if s.AgentID == "" {
		return []domain.MemoryAssertion{own}
	}
	shared := own
	shared.AgentID = ""
	shared.ID = domain.MemoryAssertionID(shared)
	return []domain.MemoryAssertion{own, shared}
}
