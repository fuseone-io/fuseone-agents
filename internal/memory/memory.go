package memory

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

type Memory struct {
	mu          sync.RWMutex
	values      map[string]domain.MemoryAssertion
	suggestions map[string]domain.MemorySuggestion
}

func NewMemory() *Memory {
	return &Memory{
		values:      map[string]domain.MemoryAssertion{},
		suggestions: map[string]domain.MemorySuggestion{},
	}
}

func (m *Memory) Find(ctx context.Context, q domain.MemoryQuery) ([]domain.MemoryAssertion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	q.Now = nowOrWall(q.Now)
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []domain.MemoryAssertion
	for _, a := range m.values {
		if memoryFindMatches(a, q) {
			out = append(out, cloneAssertion(a))
		}
	}
	sortAssertions(out)
	return first(out, domain.MemoryFindLimit(q.Limit)), nil
}

func (m *Memory) List(ctx context.Context, f Filter) ([]domain.MemoryAssertion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.Now = nowOrWall(f.Now)
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []domain.MemoryAssertion
	for _, a := range m.values {
		if listMatches(a, f) {
			out = append(out, cloneAssertion(a))
		}
	}
	sortByUpdated(out)
	return first(out, domain.MemoryListLimit(f.Limit)), nil
}

func (m *Memory) Assert(
	ctx context.Context, a domain.MemoryAssertion, by domain.UserID, reason string, now time.Time,
) (domain.MemoryAssertion, error) {
	if err := ctx.Err(); err != nil {
		return domain.MemoryAssertion{}, err
	}
	prepared, err := prepareAssertion(a, by, now)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	prepared = preserveCreatedBy(m.values[prepared.ID], prepared)
	m.values[prepared.ID] = cloneAssertion(prepared)
	return prepared, nil
}

func (m *Memory) Disable(
	ctx context.Context, id string, scope domain.Scope, by domain.UserID, reason string, now time.Time,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.values[id]
	if !ok || !scope.Contains(a.Scope) {
		return ErrNotFound
	}
	a.Status, a.UpdatedBy, a.UpdatedAt = domain.MemoryDisabled, by, now.UTC()
	m.values[id] = a
	return nil
}

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
	if out, ok := m.alreadyActiveSuggestion(prepared, now); ok {
		return out, nil
	}
	if old, ok := m.suggestions[prepared.ID]; ok {
		prepared = mergeSuggestion(old, prepared)
	}
	if prepared.Status != domain.MemorySuggestionPending {
		m.suggestions[prepared.ID] = cloneSuggestion(prepared)
		return domain.MemorySuggestionOutcome{Suggestion: prepared, Result: domain.MemorySuggestIgnored}, nil
	}
	if policy.AutoConfirms() && prepared.Observations >= policy.MinObservations {
		return m.autoConfirmSuggestion(prepared, now), nil
	}
	m.suggestions[prepared.ID] = cloneSuggestion(prepared)
	return domain.MemorySuggestionOutcome{Suggestion: prepared, Result: domain.MemorySuggestPending}, nil
}

func (m *Memory) alreadyActiveSuggestion(
	s domain.MemorySuggestion, now time.Time,
) (domain.MemorySuggestionOutcome, bool) {
	active, ok := m.values[s.AssertionID]
	if !ok || active.Status != domain.MemoryActive || active.Claim != s.Claim ||
		expired(active, nowOrWall(now)) {
		return domain.MemorySuggestionOutcome{}, false
	}
	active = cloneAssertion(active)
	return domain.MemorySuggestionOutcome{
		Suggestion: s, Assertion: &active, Result: domain.MemorySuggestAlreadyActive,
	}, true
}

func (m *Memory) autoConfirmSuggestion(s domain.MemorySuggestion, now time.Time) domain.MemorySuggestionOutcome {
	assertion := assertionFromSuggestion(s, s.Observations, domain.UserID("system:memory"), now)
	assertion = preserveCreatedBy(m.values[assertion.ID], assertion)
	m.values[assertion.ID] = cloneAssertion(assertion)
	s.Status = domain.MemorySuggestionAutoConfirmed
	s.UpdatedBy, s.UpdatedAt = assertion.UpdatedBy, assertion.UpdatedAt
	m.suggestions[s.ID] = cloneSuggestion(s)
	return domain.MemorySuggestionOutcome{
		Suggestion: s, Assertion: &assertion, Result: domain.MemorySuggestAutoConfirmed,
	}
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
	ctx context.Context, id string, scope domain.Scope, by domain.UserID, reason string, now time.Time,
) (domain.MemoryAssertion, error) {
	if err := ctx.Err(); err != nil {
		return domain.MemoryAssertion{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.suggestions[id]
	if !ok || !scope.Contains(s.Scope) || s.Status != domain.MemorySuggestionPending {
		return domain.MemoryAssertion{}, ErrNotFound
	}
	assertion := assertionFromSuggestion(s, s.Observations, by, now)
	assertion = preserveCreatedBy(m.values[assertion.ID], assertion)
	m.values[assertion.ID] = cloneAssertion(assertion)
	s.Status, s.UpdatedBy, s.UpdatedAt = domain.MemorySuggestionAccepted, by, now.UTC()
	m.suggestions[id] = cloneSuggestion(s)
	return assertion, nil
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

func preserveCreatedBy(old, next domain.MemoryAssertion) domain.MemoryAssertion {
	if old.ID == "" {
		return next
	}
	next.CreatedAt, next.CreatedBy = old.CreatedAt, old.CreatedBy
	return next
}

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
