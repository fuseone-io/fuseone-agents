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

	// Retired is whether the agent is out of circulation. It keeps its
	// versions and its runs; what it loses is every listing and the ability
	// to start.
	Retired bool

	// Started is whether the agent may open runs. Stated this way round, and
	// not as "paused", so that the zero value is the safe reading: a summary
	// nobody filled in reports an agent that cannot act, rather than showing
	// a stopped agent as live because a read failed.
	Started bool

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

/*
AgentEvent is one event a finished run publishes for composition.

The event name is what starts listeners. Context and artifacts are small
declarations about what the listener may ask for by reference; they are not a
blob of another agent's prose and not a direct call to another agent.
*/
type AgentEvent struct {
	Event     string   `json:"event" yaml:"event"`
	Context   string   `json:"context,omitempty" yaml:"context,omitempty"`
	Artifacts []string `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
}

const (
	// ToolContextRead is the platform-owned read path for governed context
	// shared by one run with another. It is a tool, not prompt text, so the
	// Gate rules on it and the trail records every use.
	ToolContextRead ToolID = "$fuseone.context.read"

	// ArtifactFinalAnswer names a run's closing answer when an event wants to
	// share it without copying the prose into the listener's input.
	ArtifactFinalAnswer = "final_answer"
)

// ContextArtifact is one claim-check a listening run may retrieve.
//
// The payload is in the content store. This contract carries only provenance,
// reference and labels, so a listener can ask for named context without being
// handed another agent's prose as instructions.
type ContextArtifact struct {
	Name        string  `json:"name"`
	Kind        string  `json:"kind,omitempty"`
	Ref         string  `json:"ref"`
	Digest      string  `json:"digest"`
	SourceRun   RunID   `json:"source_run"`
	SourceAgent AgentID `json:"source_agent,omitempty"`
	Labels      Labels  `json:"labels,omitempty"`
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

// WatchedCorpus is an agent that has corrections, and the version they are
// being checked against.
//
// A value rather than anything either side owns: the corpus store answers it
// and the drift watcher reads it, and if it lived in the watcher then the
// store would have to import the thing that consumes it.
type WatchedCorpus struct {
	Agent   AgentID
	Version VersionID
	Scope   Scope
}
