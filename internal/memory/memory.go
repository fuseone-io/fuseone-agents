package memory

import (
	"context"
	"slices"
	"strings"
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
	sortFindAssertions(out, q.Search)
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
	merged, _, err := m.mergeInto(prepared, OriginHuman)
	return merged, err
}

/*
mergeInto is the only way an assertion reaches the map, and the same decision
the durable store makes.

The mutex is already held by every caller, which is this store's whole
transaction: read, merge and write cannot interleave. What it has to match is
the order, not the mechanism — read after taking the lock, never before.
*/
func (m *Memory) mergeInto(
	incoming domain.MemoryAssertion, origin MergeOrigin,
) (domain.MemoryAssertion, MergeOutcome, error) {
	stored, covering, err := m.neighbours(incoming)
	if err != nil {
		return domain.MemoryAssertion{}, "", err
	}
	merged, outcome, err := Merge(MergeInput{
		Incoming: incoming, Stored: stored, Covering: covering,
		Origin: origin, Now: incoming.UpdatedAt,
	})
	if err != nil {
		return domain.MemoryAssertion{}, "", err
	}
	if outcome == Covered {
		return merged, outcome, nil
	}
	m.values[merged.ID] = cloneAssertion(merged)
	return merged, outcome, nil
}

// neighbours finds the row this write may merge into and the shared row that
// may already cover it, by either name an identity has.
func (m *Memory) neighbours(
	a domain.MemoryAssertion,
) (stored, covering *domain.MemoryAssertion, err error) {
	stored, err = m.byIdentity(a)
	if err != nil || a.AgentID == "" || stored != nil {
		return stored, nil, err
	}
	shared := a
	shared.AgentID = ""
	covering, err = m.byIdentity(shared)
	return nil, covering, err
}

// byIdentity applies the same rule the durable store applies, from the same
// place: more than one row is this identity and there is no answer to give.
//
// Sorted before the rule, because a map has no order and a refusal that named a
// different pair each time would be a refusal nobody could act on.
func (m *Memory) byIdentity(a domain.MemoryAssertion) (*domain.MemoryAssertion, error) {
	key := domain.CanonicalIdentityKey(a)
	var found []domain.MemoryAssertion
	for _, held := range m.values {
		if held.Scope != a.Scope || held.AgentID != a.AgentID {
			continue
		}
		if held.ID == a.ID || domain.CanonicalIdentityKey(held) == key {
			found = append(found, cloneAssertion(held))
		}
	}
	slices.SortFunc(found, func(x, y domain.MemoryAssertion) int {
		return strings.Compare(x.ID, y.ID)
	})
	return oneOf(found, key)
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
	if policy.AutoConfirms(prepared.Labels) && prepared.Observations >= policy.MinObservations {
		return m.autoConfirmSuggestion(prepared, now)
	}
	m.suggestions[prepared.ID] = cloneSuggestion(prepared)
	return domain.MemorySuggestionOutcome{Suggestion: prepared, Result: domain.MemorySuggestPending}, nil
}

func (m *Memory) alreadyActiveSuggestion(
	s domain.MemorySuggestion, now time.Time,
) (domain.MemorySuggestionOutcome, bool) {
	for _, id := range activeAssertionIDsForSuggestion(s) {
		active, ok := m.values[id]
		if !ok || active.Status != domain.MemoryActive || expired(active, nowOrWall(now)) {
			continue
		}
		active = cloneAssertion(active)
		return domain.MemorySuggestionOutcome{
			Suggestion: s, Assertion: &active, Result: domain.MemorySuggestAlreadyActive,
		}, true
	}
	return domain.MemorySuggestionOutcome{}, false
}

func (m *Memory) autoConfirmSuggestion(
	s domain.MemorySuggestion, now time.Time,
) (domain.MemorySuggestionOutcome, error) {
	assertion := assertionFromSuggestion(s, s.Observations, domain.UserID("system:memory"), now)
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
