package ledger

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
)

// EventDispatchBacklog holds the Postgres contract: the real finished runs
// past the cursor and when the oldest finished. Every append here completes
// as it happens, so no transaction is ever in flight to hold a run back:
// everything waiting is Ready.
func (m *Memory) EventDispatchBacklog(ctx context.Context) (domain.EventDispatchBacklog, error) {
	if err := ctx.Err(); err != nil {
		return domain.EventDispatchBacklog{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.eventDispatchBacklog(), nil
}

// eventDispatchBacklog reads under the caller's lock.
func (m *Memory) eventDispatchBacklog() domain.EventDispatchBacklog {
	var out domain.EventDispatchBacklog
	for id, order := range m.finishOrder {
		steps := m.runs[id]
		if !m.eventCursor.Before(domain.FinishCursor{Xid: order, Run: id}) || isSimulated(steps) {
			continue
		}
		summary := summarise(steps)
		if summary.Phase != "finished" {
			continue
		}
		out.Ready++
		if out.OldestFinishedAt.IsZero() || summary.EndedAt.Before(out.OldestFinishedAt) {
			out.OldestFinishedAt = summary.EndedAt
		}
	}
	return out
}
