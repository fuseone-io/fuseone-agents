package memory

import (
	"context"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// The fake's queue of proposals, kept apart from its memory of facts.
//
// They share one mutex and nothing else: a suggestion is something an agent
// noticed and nobody has agreed to yet, and every path out of here that does
// agree goes through mergeInto rather than writing a row of its own.

func (m *Memory) Suggest(
	ctx context.Context, s domain.MemorySuggestion, policy domain.MemoryLearningPolicy,
	by domain.UserID, now time.Time,
) (domain.MemorySuggestionOutcome, error) {
	if err := ctx.Err(); err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	policy = policy.Normalize()
	if !policy.Enabled() {
		return domain.MemorySuggestionOutcome{Result: domain.MemorySuggestIgnored}, nil
	}
	prepared, err := prepareSuggestion(s, by, now)
	if err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if out, done, err := m.alreadyActiveSuggestion(prepared, now); done || err != nil {
		return out, err
	}
	if old, ok := m.suggestions[prepared.ID]; ok {
		prepared = mergeSuggestion(old, prepared)
	}
	if prepared.Status != domain.MemorySuggestionPending {
		m.suggestions[prepared.ID] = cloneSuggestion(prepared)
		return domain.MemorySuggestionOutcome{Suggestion: prepared, Result: domain.MemorySuggestIgnored}, nil
	}
	if policy.AutoConfirms(prepared.Labels) && prepared.Observations >= policy.MinObservations {
		return m.autoConfirmSuggestion(prepared, now)
	}
	m.suggestions[prepared.ID] = cloneSuggestion(prepared)
	return domain.MemorySuggestionOutcome{Suggestion: prepared, Result: domain.MemorySuggestPending}, nil
}

func (m *Memory) alreadyActiveSuggestion(
	s domain.MemorySuggestion, now time.Time,
) (domain.MemorySuggestionOutcome, bool, error) {
	for _, identity := range identitiesForSuggestion(s) {
		active, err := m.byIdentity(identity)
		if err != nil {
			return domain.MemorySuggestionOutcome{}, false, err
		}
		if active == nil || active.Status != domain.MemoryActive ||
			expired(*active, nowOrWall(now)) {
			continue
		}
		return domain.MemorySuggestionOutcome{
			Suggestion: s, Assertion: active, Result: domain.MemorySuggestAlreadyActive,
		}, true, nil
	}
	return domain.MemorySuggestionOutcome{}, false, nil
}

func (m *Memory) autoConfirmSuggestion(
	s domain.MemorySuggestion, now time.Time,
) (domain.MemorySuggestionOutcome, error) {
	assertion := assertionFromSuggestion(s, s.Observations, systemMemory, now)
	merged, outcome, err := m.mergeInto(assertion, OriginAutoConfirm)
	if err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if outcome == Covered {
		s.Status, s.CoveredBy = domain.MemorySuggestionCovered, merged.ID
		s.UpdatedBy, s.UpdatedAt = merged.UpdatedBy, merged.UpdatedAt
		m.suggestions[s.ID] = cloneSuggestion(s)
		return domain.MemorySuggestionOutcome{
			Suggestion: s, Assertion: &merged, Result: domain.MemorySuggestAlreadyActive,
		}, nil
	}
	s.Status = domain.MemorySuggestionAutoConfirmed
	s.UpdatedBy, s.UpdatedAt = merged.UpdatedBy, merged.UpdatedAt
	m.suggestions[s.ID] = cloneSuggestion(s)
	return domain.MemorySuggestionOutcome{
		Suggestion: s, Assertion: &merged, Result: domain.MemorySuggestAutoConfirmed,
	}, nil
}

func (m *Memory) ListSuggestions(ctx context.Context, f SuggestionFilter) ([]domain.MemorySuggestion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []domain.MemorySuggestion
	for _, s := range m.suggestions {
		if suggestionMatches(s, f) {
			out = append(out, cloneSuggestion(s))
		}
	}
	sortSuggestionsByUpdated(out)
	return firstSuggestions(out, domain.MemorySuggestLimit(f.Limit)), nil
}

func (m *Memory) AcceptSuggestion(
	ctx context.Context, in AcceptInput,
) (domain.MemoryAssertion, error) {
	if err := ctx.Err(); err != nil {
		return domain.MemoryAssertion{}, err
	}
	if err := in.validate(); err != nil {
		return domain.MemoryAssertion{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.suggestions[in.ID]
	if !ok || !in.Scope.Contains(s.Scope) || s.Status != domain.MemorySuggestionPending {
		return domain.MemoryAssertion{}, ErrNotFound
	}
	assertion, err := accepted(s, in)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	by, now, id := in.By, in.Now, in.ID
	merged, outcome, err := m.mergeInto(assertion, OriginAccept)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	if outcome == Covered {
		// The proposal is finished even though nothing was written: memory that
		// was already there answered it. Leaving it pending would be a queue
		// item with no honest exit; dismissing it would record a refusal nobody
		// made.
		s.Status, s.CoveredBy = domain.MemorySuggestionCovered, merged.ID
		s.UpdatedBy, s.UpdatedAt = by, now.UTC()
		m.suggestions[id] = cloneSuggestion(s)
		return merged, nil
	}
	// After the merge: a suggestion marked accepted beside an assertion that
	// was never written is a queue that empties while nothing is learned.
	s.Status, s.UpdatedBy, s.UpdatedAt = domain.MemorySuggestionAccepted, by, now.UTC()
	m.suggestions[id] = cloneSuggestion(s)
	return merged, nil
}

func (m *Memory) DismissSuggestion(
	ctx context.Context, id string, scope domain.Scope, by domain.UserID, reason string, now time.Time,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.suggestions[id]
	if !ok || !scope.Contains(s.Scope) || s.Status != domain.MemorySuggestionPending {
		return ErrNotFound
	}
	s.Status, s.UpdatedBy, s.UpdatedAt = domain.MemorySuggestionDismissed, by, now.UTC()
	m.suggestions[id] = cloneSuggestion(s)
	return nil
}
