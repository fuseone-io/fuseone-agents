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
	merged, outcome, err := m.mergeInto(prepared, OriginHuman)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	if outcome == Covered {
		return domain.MemoryAssertion{}, coveredBy(merged)
	}
	return merged, nil
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
