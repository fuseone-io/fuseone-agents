package ledger

import (
	"context"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// EventDispatchBacklog holds the Postgres contract: the real finished runs
// past the cursor, counted up to the cap, and the earliest finish among them
// — by the clock that stamped each, not by the feed's order. Every append here completes
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
		if at := finishedAt(summary); out.OldestFinishedAt.IsZero() || at.Before(out.OldestFinishedAt) {
			out.OldestFinishedAt = at
		}
	}
	return capBacklog(out)
}

// finishedAt is when a finished run stopped: its end, or its last write when
// nothing recorded an end — as Postgres reads coalesce(ended_at, updated_at).
func finishedAt(summary domain.RunSummary) time.Time {
	if summary.EndedAt.IsZero() {
		return summary.UpdatedAt
	}
	return summary.EndedAt
}
