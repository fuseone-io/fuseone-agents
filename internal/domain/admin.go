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
	// CompensatedBy is the tool that takes this one back, when the Curator has
	// said which does. Empty means an act by this tool cannot be undone by
	// machine, which the abandonment screen reports rather than hides.
	CompensatedBy ToolID
	// Suggested is what the platform ships about a server it knows, and it is
	// not a classification: it is the first proposal, with its reasoning, so
	// the Curator confirms instead of inventing. Nil for a server nobody
	// catalogued and for a tool an entry never heard of.
	Suggested *ToolSuggestion
	// Digest names the definition on offer right now. It travels to the screen
	// so a ruling made there can say which definition it judged, the way an
	// approval says which step it approved.
	Digest string
	// Stale marks a ruling overtaken by a new definition. Refused like an
	// unruled tool and shown differently: one is a decision to make, the other
	// a decision to check.
	Stale bool
}

// ToolSuggestion is a shipped opinion about one tool, and the sentence behind
// it.
//
// The sentence is not decoration. A suggested classification with no reasoning
// is a number to click past, and clicking past is exactly the failure a
// suggestion invites — it looks like work already done.
type ToolSuggestion struct {
	Effect        Effect
	Untrusted     bool
	CompensatedBy ToolID
	Why           string
}

// MCPServer and ModelProvider are the configured integrations as a reader sees
// them. They live here for the same reason ToolClassification does: the
// administration that writes them and the API that renders them must not
// import each other.
// Transports a tool server can be reached over.
//
// A server is either a process this installation runs or an address it calls.
// The difference is not cosmetic: a command with arguments is code executed
// inside the worker's container, which is a far larger thing to hand somebody
// than a URL.
const (
	TransportStdio = "stdio"
	TransportHTTP  = "http"
)

type MCPServer struct {
	Name string
	// Transport is stdio or http. Empty reads as stdio: rows written before
	// the field existed are the commands they always were, and defaulting them
	// to anything else would stop an installation's tools connecting on
	// upgrade.
	Transport string

	// Command and Args are the local process, for stdio.
	Command string
	Args    []string

	// URL is the endpoint, for http.
	URL string
	// HasSecret reports that a bearer token is stored, never what it is.
	HasSecret bool

	// AcceptsLocalExecution records that somebody accepted what stdio is.
	//
	// A local server is a program this installation starts inside the worker,
	// running as the worker, on its filesystem, from inside its network. The
	// Gate decides what a tool may do and decides nothing about what a process
	// may read — so this is not a control against a hostile administrator, and
	// claiming it were would be worse than not having it. It is informed
	// consent and a record of who gave it, which is what an installation can
	// honestly offer for a decision only a person can make.
	AcceptsLocalExecution bool

	Enabled   bool
	UpdatedBy string
	UpdatedAt time.Time
}

// TransportOf reads a server's transport, defaulting an unset one to stdio.
func (s MCPServer) TransportOf() string {
	if s.Transport == "" {
		return TransportStdio
	}
	return s.Transport
}

// IdentityProvider is one configured way of signing in.
//
// Authenticating and being allowed to do something are separate: the mappings
// are what turn an assertion's groups into scoped grants, and a provider with
// none grants nothing however successfully somebody signs in.
type IdentityProvider struct {
	ID      string
	Display string
	Issuer  string
	// ClientID names this installation to the provider. The secret beside it
	// lives in the vault and is never returned by a listing.
	ClientID     string
	ClientSecret string
	// HasSecret reports that a credential is stored, never what it is.
	HasSecret bool
	// GroupsClaim names the claim carrying group membership. Providers differ:
	// Keycloak and Okta commonly use "groups", Entra ID "roles".
	GroupsClaim string
	Mappings    []GroupMapping
	Enabled     bool
	UpdatedBy   string
	UpdatedAt   time.Time
}

// GroupMapping turns a group asserted by the provider into a scoped grant.
type GroupMapping struct {
	Group   string `json:"group"`
	Company string `json:"company"`
	Area    string `json:"area"`
	Role    string `json:"role"`
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
