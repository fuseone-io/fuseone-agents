package domain

import "time"

/*
EventDispatchBacklog is what the event dispatcher has not reached yet (#228).

The dispatcher reads finished runs forward from a durable cursor, and only
below the oldest transaction still in flight (pg_snapshot_xmin), so a long
transaction anywhere in the database delays every dispatch while it is open.
That is the intended trade-off — it delays, never skips — and this is what
makes the delay visible: how much is waiting, how long the oldest of it has
waited, and whether the wait is the dispatcher's or an open transaction's.

Counts and instants only. No run, agent or transaction id: these become
metric samples and a health card, and neither may name what they count.
*/
type EventDispatchBacklog struct {
	// Ready is the finished runs past the cursor the feed can read now: the
	// dispatcher's own backlog, cleared by its next sweep.
	Ready int64
	// HeldByXmin is the finished runs past the cursor that committed at or
	// above the oldest transaction still in flight. The feed cannot read
	// them until that transaction ends, however often it sweeps.
	HeldByXmin int64
	// OldestFinishedAt is when the oldest run past the cursor finished;
	// zero when nothing is waiting.
	OldestFinishedAt time.Time
	// Xact is the oldest transaction holding an id — the one that sets xmin.
	Xact OpenXact
}

// OpenXact is the oldest open transaction that holds a transaction id.
type OpenXact struct {
	// Open reports that one exists.
	Open bool
	// StartedAt is when it began; zero when the database role the platform
	// connects as may not see other sessions' transactions.
	StartedAt time.Time
}

// EventDispatchDegradedAfter is how long the oldest finished run may wait
// before the health signal degrades. A sweep runs every fifteen seconds, so a
// healthy dispatcher never comes near it; five minutes is a transaction that
// stayed open, or a dispatcher that stopped, not a slow sweep.
const EventDispatchDegradedAfter = 5 * time.Minute

// The health vocabulary. Degraded, never failing: dispatch is delayed, and
// nothing is lost while it is.
const (
	EventDispatchOK       = "ok"
	EventDispatchDegraded = "degraded"
)

// Lag is how long the oldest finished run past the cursor has waited at now.
// Never negative: a worker stamps a finish with its own clock, and one ahead
// of ours is not a run that has waited less than nothing.
func (b EventDispatchBacklog) Lag(now time.Time) time.Duration {
	if b.OldestFinishedAt.IsZero() || !now.After(b.OldestFinishedAt) {
		return 0
	}
	return now.Sub(b.OldestFinishedAt)
}

// Held reports that committed runs wait on xmin rather than on a sweep.
func (b EventDispatchBacklog) Held() bool { return b.HeldByXmin > 0 }

// XactAge is how long the oldest id-holding transaction has been open; zero
// and known when none is. Unknown when one is open and the role may not see
// when it began.
func (b EventDispatchBacklog) XactAge(now time.Time) (time.Duration, bool) {
	switch {
	case !b.Xact.Open:
		return 0, true
	case b.Xact.StartedAt.IsZero():
		return 0, false
	case !now.After(b.Xact.StartedAt):
		return 0, true
	}
	return now.Sub(b.Xact.StartedAt), true
}

// Status is the health signal at now: degraded once the oldest finished run
// has waited past EventDispatchDegradedAfter, whichever the cause.
func (b EventDispatchBacklog) Status(now time.Time) string {
	if b.Lag(now) > EventDispatchDegradedAfter {
		return EventDispatchDegraded
	}
	return EventDispatchOK
}
