package domain

import "time"

// AgentSummary is a published agent version as a list shows it.
//
// Read from the registry rather than from a directory on somebody's disk: a
// run is pinned to a version, and an auditor reading a two-year-old run needs
// the exact text it ran under, not whatever the file says today.
type AgentSummary struct {
	// Stage is how far this agent is trusted to act alone. State beside the
	// specification, so it is filled in by whoever reads it rather than
	// carried by the published version.
	Stage Stage

	ID        AgentID
	VersionID VersionID
	Scope     Scope
	Name      string

	Provider string
	Model    string
	Effort   string

	Tools    []ToolID
	Budget   Budget
	Triggers []AgentTrigger

	PublishedBy UserID
	PublishedAt time.Time

	// Latest reports whether this is the newest published version of the
	// agent. A list shows one row per agent by default; the history is asked
	// for separately.
	Latest bool
}

// AgentTrigger is what starts a run: a schedule, a webhook path, an event.
type AgentTrigger struct {
	Type     string `json:"type"`
	Schedule string `json:"schedule,omitempty"`
	Path     string `json:"path,omitempty"`
	Event    string `json:"event,omitempty"`
}

// AgentActivity is how an agent has been doing, aggregated from its runs.
//
// An agent has no state of its own to report — the platform has no autonomy
// stage yet, and inventing one would put a field on the screen that nothing
// maintains. What an operator actually wants to know is whether it is running,
// stuck, or has never run, and that is a fact about its runs.
type AgentActivity struct {
	AgentID AgentID

	Runs     int64
	Finished int64
	// Waiting counts runs stopped for a person: awaiting approval or parked.
	Waiting    int64
	CostMicros int64

	// LastPhase is the phase of the most recent run. Empty when the agent has
	// never run, which is a different thing from having run and finished.
	LastPhase string
	LastRunAt time.Time
}

// SuccessRate is finished over total, or -1 when nothing has run. Not zero:
// zero is a measurement, and there is nothing to measure.
func (a AgentActivity) SuccessRate() float64 {
	if a.Runs == 0 {
		return -1
	}
	return float64(a.Finished) / float64(a.Runs)
}

/*
EventEdge is one link in the composition graph (PRD SE-10).

An edge with no To is an event nobody listens to; an edge with no From is a
trigger nothing publishes. Both are kept, because they are the two mistakes
this graph exists to make visible and a picture without them looks correct.
*/
type EventEdge struct {
	From  AgentID
	Event string
	To    AgentID
}
