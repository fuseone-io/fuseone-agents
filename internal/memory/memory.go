package memory

import (
	"context"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

type Memory struct {
	mu     sync.RWMutex
	values map[string]domain.MemoryAssertion
}

func NewMemory() *Memory { return &Memory{values: map[string]domain.MemoryAssertion{}} }

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

func preserveCreatedBy(old, next domain.MemoryAssertion) domain.MemoryAssertion {
	if old.ID == "" {
		return next
	}
	next.CreatedAt, next.CreatedBy = old.CreatedAt, old.CreatedBy
	return next
}
