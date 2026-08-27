package memory

import (
	"slices"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// What a proposal becomes, for both stores.
//
// Neither of these belongs to the fake or to Postgres: the durable store folds
// a repeated observation with mergeSuggestion and builds the assertion it is
// about to write with assertionFromSuggestion, and so does the fake. Two copies
// would be two answers to "is this the same sighting", and the one that drifted
// would be whichever has fewer tests.

func mergeSuggestion(old, next domain.MemorySuggestion) domain.MemorySuggestion {
	if old.Status != domain.MemorySuggestionPending {
		return old
	}
	next.CreatedAt, next.CreatedBy = old.CreatedAt, old.CreatedBy
	next.Observations = old.Observations
	if hasNewEvidenceRun(old.Evidence, next.Evidence) {
		next.Observations++
	}
	next.Labels = old.Labels.Union(next.Labels)
	next.Evidence = boundedEvidence(append(old.Evidence, next.Evidence...))
	return next
}

func hasNewEvidenceRun(old, next []domain.MemoryEvidence) bool {
	seen := map[domain.RunID]bool{}
	for _, ev := range old {
		seen[ev.RunID] = true
	}
	for _, ev := range next {
		if !seen[ev.RunID] {
			return true
		}
	}
	return false
}

func assertionFromSuggestion(
	s domain.MemorySuggestion, confirmed int64, by domain.UserID, now time.Time,
) domain.MemoryAssertion {
	return domain.MemoryAssertion{
		ID: s.AssertionID, Scope: s.Scope, AgentID: s.AgentID,
		Kind: s.Kind, Subject: s.Subject, Signature: s.Signature, Claim: s.Claim,
		Evidence: slices.Clone(s.Evidence), Observations: s.Observations,
		Confirmed: confirmed, Labels: s.Labels.Clone(), Status: domain.MemoryActive,
		ExpiresAt: s.ExpiresAt, CreatedBy: by, CreatedAt: now.UTC(),
		UpdatedBy: by, UpdatedAt: now.UTC(),
	}
}
