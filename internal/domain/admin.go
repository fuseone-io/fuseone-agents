package domain

import (
	"encoding/json"
	"time"
)

// AdminEvent is one recorded change to the platform itself.
type AdminEvent struct {
	At        time.Time
	Principal UserID
	Scope     Scope
	Action    string
	Target    string
	Detail    json.RawMessage
}

// ToolEntry is a tool as the administration area reads it: what it is, where
// it came from, and what somebody decided it does.
type ToolEntry struct {
	ID          ToolID
	Server      string
	Description string
	Effect      Effect
	Untrusted   bool
}

// MCPServer and ModelProvider are the configured integrations as a reader sees
// them. They live here for the same reason ToolClassification does: the
// administration that writes them and the API that renders them must not
// import each other.
type MCPServer struct {
	Name      string
	Command   string
	Args      []string
	Enabled   bool
	UpdatedBy string
	UpdatedAt time.Time
}

type ModelProvider struct {
	Name    string
	Kind    string
	BaseURL string
	Enabled bool
	// HasKey reports that a credential is stored, never what it is.
	HasKey    bool
	UpdatedBy string
	UpdatedAt time.Time
}

// IntegrationHealth is what the platform observed about a connected system the
// last time it tried to reach it.
//
// Observation, not configuration. A server can be enabled, correct and
// unreachable, and only one of those three is somebody's opinion.
type IntegrationHealth struct {
	Name      string
	Reachable bool
	// ToolCount is how many tools it offered. Zero on an unreachable server,
	// and also on a reachable one that offers nothing — which is why Reachable
	// is a separate field rather than inferred from this.
	ToolCount int
	// Detail is why it failed, when it did. Developer-facing and shown as-is:
	// the person reading it is the one who has to fix the server.
	Detail     string
	ObservedAt time.Time
	// ObservedBy is which worker saw this. Several connect to the same servers
	// and can disagree — one pod on a network that reaches it, one not.
	ObservedBy string
}
