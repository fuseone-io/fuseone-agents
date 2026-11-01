package domain

import "strings"

/*
FinishCursor is a position in the order finished runs were committed in:
the finishing transaction, then the run, for the rare runs one transaction
finished together.

The event dispatcher reads finished runs from it and moves it forward only
once every run up to a position has been dispatched, so a run that finishes
between two sweeps — or a burst larger than one page — is never left behind
(#203). Commit order rather than a clock: a run stamped earlier can commit
later, and a cursor that had already passed its timestamp would never see it.
*/
type FinishCursor struct {
	Xid uint64
	Run RunID
}

// Before reports whether c comes before o in commit order.
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

// FinishedRun is a run that finished, and where it stands in commit order.
type FinishedRun struct {
	Run RunSummary
	At  FinishCursor
}
