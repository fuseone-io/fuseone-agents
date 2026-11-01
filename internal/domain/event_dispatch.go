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
	// OldestFinishedAt is the earliest finish among the runs past the
	// cursor that were read; zero when nothing is waiting. When a count
	// is capped, only a sample was read and the true earliest may be
	// earlier still.
	OldestFinishedAt time.Time
	// Xact is the oldest transaction holding an id — the one that sets xmin.
	Xact OpenXact
	// ReadyCapped and HeldCapped report that Ready, or HeldByXmin, stopped
	// counting at EventDispatchCountCap: that count is at least the cap.
	// Each says which count was cut, so exactly a thousand reads as a
	// thousand and only the cut one as more.
	ReadyCapped bool
	HeldCapped  bool
}

// Capped reports that either count stopped at the cap.
func (b EventDispatchBacklog) Capped() bool { return b.ReadyCapped || b.HeldCapped }

// EventDispatchCountCap is where the backlog counts stop. The backlog is read
// every minute on every worker and on every health page view, during the very
// incident it describes; a count that walked the whole range would grow with
// the incident. A thousand runs waiting already says everything a number can.
const EventDispatchCountCap = 1000

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

/*
Status is the health signal at now: degraded once the oldest finished run has
waited past EventDispatchDegradedAfter, whichever the cause — and degraded
whenever a count was capped.

A capped count means the lag was read from a bounded sample of the backlog,
so it is only a lower bound: an older run may sit beyond the sample. Reading
that as healthy would be the one wrong answer. And a thousand finished runs
not yet dispatched is itself the condition the signal exists for: the
dispatcher drains every readable page on each fifteen-second sweep, so a
backlog that large is either held by xmin or a dispatcher not keeping up.
*/
func (b EventDispatchBacklog) Status(now time.Time) string {
	if b.Capped() || b.Lag(now) > EventDispatchDegradedAfter {
		return EventDispatchDegraded
	}
	return EventDispatchOK
}
