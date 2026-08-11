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
