package ledger

import (
	"context"
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

// FinishedSince holds the Postgres contract: real finished runs after the
// cursor, in the order they finished, a page at a time. Every append here
// completes as it happens, so there is never a transaction in flight to wait
// for, and the order runs finished in is the feed's order.
func (m *Memory) FinishedSince(
	ctx context.Context, after domain.FinishCursor, limit int,
) ([]domain.FinishedRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []domain.FinishedRun
	for id, order := range m.finishOrder {
		steps := m.runs[id]
		at := domain.FinishCursor{Xid: order, Run: id}
		if !after.Before(at) || isSimulated(steps) {
			continue
		}
		if summary := summarise(steps); summary.Phase == "finished" {
			out = append(out, domain.FinishedRun{Run: summary, At: at})
		}
	}
	slices.SortFunc(out, func(a, b domain.FinishedRun) int {
		switch {
		case a.At.Before(b.At):
			return -1
		case b.At.Before(a.At):
			return 1
		}
		return 0
	})
	if n := limitOrDefault(limit); len(out) > n {
		out = out[:n]
	}
	return out, nil
}

// EventCursor is where the dispatcher has got to.
func (m *Memory) EventCursor(ctx context.Context) (domain.FinishCursor, error) {
	if err := ctx.Err(); err != nil {
		return domain.FinishCursor{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.eventCursor, nil
}

// AdvanceEventCursor keeps the furthest of the cursor and to, as the
// Postgres compare-and-write does.
func (m *Memory) AdvanceEventCursor(ctx context.Context, to domain.FinishCursor) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.eventCursor.Before(to) {
		m.eventCursor = to
	}
	return nil
}
