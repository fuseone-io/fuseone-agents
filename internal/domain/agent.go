package domain

import "time"

// AgentSummary is a published agent version as a list shows it.
//
// Read from the registry rather than from a directory on somebody's disk: a
// run is pinned to a version, and an auditor reading a two-year-old run needs
// the exact text it ran under, not whatever the file says today.
type AgentSummary struct {
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
