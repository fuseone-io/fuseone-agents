package domain

import "strings"

/*
FinishCursor is a position among finished runs: the id of the transaction
that finished the run, then the run, for the rare runs one transaction
finished together.

Transaction ids are handed out in the order transactions start, not the order
they commit, so the order alone would not do: a transaction with a lower id
can commit after a higher one was read. The feed is therefore read only below
the oldest transaction still in flight (pg_snapshot_xmin) — a boundary below
which every transaction has finished — and within it this order is stable:
nothing can appear behind a position once the feed has passed it (#203).
A clock would not do either: a run stamped earlier can commit later.

The event dispatcher reads finished runs from it and moves it forward only
once every run up to a position has been dispatched, so a run that finishes
between two sweeps — or a burst larger than one page — is never left behind.
*/
type FinishCursor struct {
	Xid uint64
	Run RunID
}

// Before reports whether c comes before o in the feed.
func (c FinishCursor) Before(o FinishCursor) bool {
	if c.Xid != o.Xid {
		return c.Xid < o.Xid
	}
	return strings.Compare(string(c.Run), string(o.Run)) < 0
}

// EventListener is an agent an event starts, and the published version that
// declared the trigger. A run is opened on that version: a later publish
// that stops listening must not start the version that never asked.
type EventListener struct {
	Agent   AgentID
	Version VersionID
}

// FinishedRun is a run that finished, and where it stands in the feed.
type FinishedRun struct {
	Run RunSummary
	At  FinishCursor
}
