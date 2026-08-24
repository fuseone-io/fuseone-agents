package domain

import (
	"encoding/json"
	"regexp"
	"time"
)

// AdminEvent is one recorded change to the platform itself.
type AdminEvent struct {
	// ID is the append-only row position. It never leaves the API; it exists so
	// the administrative trail can page without offset drift.
	ID        int64
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
	// OnSurface is whether this installation brought the tool in. Off it, the
	// tool is not a capability here — the administration area still lists it,
	// because "discovered and not taken" is a state somebody chose.
	OnSurface bool
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

// MCP protocol modes control how a Streamable HTTP server is negotiated.
const (
	MCPProtocolAuto   = "auto"
	MCPProtocolLegacy = "legacy"
)

// DefaultMCPConfigFileEnv is where a local server receives the path of a
// platform-managed configuration file unless an operator names a different
// variable for that server.
const DefaultMCPConfigFileEnv = "FUSEONE_MCP_CONFIG_FILE"

// MCPRateLimit bounds outgoing tool calls to one MCP server from one worker
// process. It is deliberately local to the process: a distributed limiter
// would be a separate coordination system, while this is an operator's guard
// against bursts into a fragile integration.
type MCPRateLimit struct {
	RatePerSecond float64
	Burst         int
}

// MCPResultCache keeps successful read results for a short time inside one
// worker process. It is an optimisation, not a record: every cached hit still
// writes a fresh content reference into the run that used it.
type MCPResultCache struct {
	TTLSeconds int
	MaxEntries int
}

const (
	MCPEgressInherit = "inherit"
	MCPEgressProxied = "proxied"
)

type MCPEgressDestination struct {
	Host string
	Port int
}

var mcpEgressHostPattern = regexp.MustCompile(
	`^(?:\*\.)?[a-z0-9](?:[a-z0-9_.-]{0,251}[a-z0-9])?$`,
)

func ValidMCPEgressDestination(dest MCPEgressDestination) bool {
	return dest.Port > 0 && dest.Port <= 65535 &&
		dest.Host != "" && mcpEgressHostPattern.MatchString(dest.Host)
}

type MCPStdioEgress struct {
	Mode                string
	AllowedDestinations []MCPEgressDestination
}

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
	// ProtocolMode is normally auto. Legacy forces the pre-2026 initialize
	// handshake for servers whose endpoint rejects the newer server/discover
	// probe rather than negotiating it.
	ProtocolMode string
	// HasSecret reports that a credential document is stored, never what it is.
	HasSecret bool
	// HasOAuth reports that the credential document includes an OAuth grant
	// for a remote server. The grant itself never comes back through the API.
	HasOAuth bool
	// HasVariables reports that the credential document includes environment
	// variables for a local process.
	HasVariables bool
	// HasConfigFile reports that the credential document includes a managed
	// configuration file. The content never comes back through the API.
	HasConfigFile bool
	// ConfigFileEnv is the environment variable that receives the managed
	// configuration file path. Nil means the default variable.
	ConfigFileEnv *string

	/*
		Surface is which of the server's tools this installation brought in.

		Remote names, as the server calls them, because that is what survives a
		reconnect — the platform's own identifier is derived from it.

		Nil is not empty, and the difference is an upgrade: a server whose
		surface nobody has chosen goes on offering what it always did, and one
		chosen as empty offers nothing. A tool appearing later on a server with
		a chosen surface arrives outside it, for the reason a new tool arrives
		unclassified.
	*/
	Surface *[]string

	// RateLimit bounds tool calls sent from one worker process to this server.
	// Nil means no limit. With multiple workers, each worker has its own bucket.
	RateLimit *MCPRateLimit
	// Cache keeps successful read results inside one worker process. Nil means
	// no cache. With multiple workers, each process keeps its own entries.
	Cache *MCPResultCache
	// StdioEgress is meaningful only for stdio. Nil reads as inherit: the
	// local process runs with the worker's network reach and no platform
	// egress proxy is requested.
	StdioEgress *MCPStdioEgress

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

// MCPPersonalCredential is the read model for one person's credential to one
// remote MCP server. It reports presence and shape only; the sealed material
// never leaves the vault through an ordinary read.
type MCPPersonalCredential struct {
	Server string
	// Principal is the user who owns the credential. The API only lists the
	// caller's own rows, but the store keeps the owner explicit because the
	// vault key is shared infrastructure, not a browser session.
	Principal  UserID
	HasSecret  bool
	HasHeaders bool
	HasOAuth   bool
	UpdatedBy  string
	UpdatedAt  time.Time
}

// TransportOf reads a server's transport, defaulting an unset one to stdio.
func (s MCPServer) TransportOf() string {
	if s.Transport == "" {
		return TransportStdio
	}
	return s.Transport
}

// MCPProtocolModeOf reads the HTTP protocol negotiation mode.
func (s MCPServer) MCPProtocolModeOf() string {
	if s.ProtocolMode == "" {
		return MCPProtocolAuto
	}
	return s.ProtocolMode
}

// ConfigFileEnvName returns the variable that receives the managed config path.
func (s MCPServer) ConfigFileEnvName() string {
	if s.ConfigFileEnv != nil && *s.ConfigFileEnv != "" {
		return *s.ConfigFileEnv
	}
	return DefaultMCPConfigFileEnv
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
	// LastReachableAt is the last successful discovery. A failed probe must not
	// erase it: "it worked before and fails now" is more useful than just "it
	// fails".
	LastReachableAt *time.Time
	// ToolCall is the last tools/call observation. Discovery and tool calls use
	// different credentials and failure modes, so one cannot stand in for the
	// other.
	ToolCall *IntegrationToolCallHealth
}

// IntegrationToolCallHealth is what a concrete tools/call attempt proved.
//
// It intentionally carries a stable code rather than the raw error. Tool-call
// failures can include third-party URLs or diagnostics; the integration page
// needs the family, not the payload.
type IntegrationToolCallHealth struct {
	OK         bool
	Code       string
	ObservedAt time.Time
	ObservedBy string
	LastOKAt   *time.Time
}

// IntegrationToolCallObservation is the write shape for one tools/call
// attempt.
type IntegrationToolCallObservation struct {
	Name       string
	OK         bool
	Code       string
	ObservedAt time.Time
	ObservedBy string
}
