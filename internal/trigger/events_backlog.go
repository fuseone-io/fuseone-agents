package trigger

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// BacklogReader is what the dispatcher has not reached (#228), and the
// transaction that may be holding it back.
type BacklogReader interface {
	EventDispatchBacklog(ctx context.Context) (domain.EventDispatchBacklog, error)
}

// BacklogSink records each observation. Its signature carries counts and
// instants only, so nothing it receives could become a high-cardinality label.
type BacklogSink interface {
	EventDispatch(b domain.EventDispatchBacklog, now time.Time)
	EventDispatchUnobserved()
}

/*
BacklogWatch makes the dispatch delay visible. The dispatcher reads only
below pg_snapshot_xmin, so a long transaction anywhere in the database
delays every dispatch while it is open: delayed, never skipped, and until
this, invisible.

A periodic query rather than a durable record: the metrics registry keeps
the latest reading and Prometheus the history.
*/
type BacklogWatch struct {
	feed  BacklogReader
	sink  BacklogSink
	clock Clock
}

func NewBacklogWatch(feed BacklogReader, sink BacklogSink, clock Clock) *BacklogWatch {
	return &BacklogWatch{feed: feed, sink: sink, clock: clock}
}

// Observe reads the backlog once and records it at the clock's instant. A
// failed read is counted, and the last good reading stays where it was.
func (w *BacklogWatch) Observe(ctx context.Context) error {
	backlog, err := w.feed.EventDispatchBacklog(ctx)
	if err != nil {
		w.sink.EventDispatchUnobserved()
		return fmt.Errorf("trigger: read event dispatch backlog: %w", err)
	}
	w.sink.EventDispatch(backlog, w.clock.Now())
	return nil
}
