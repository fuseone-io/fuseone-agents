package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/netguard"
	"github.com/fuseone/agents/internal/settings"
)

var (
	ErrNoName        = errors.New("admin: an integration needs a name")
	ErrNoURL         = errors.New("admin: a remote tool server needs an address")
	ErrBadMCPURL     = errors.New("admin: a remote tool server address must be http or https")
	ErrBlockedMCPURL = errors.New(
		"admin: a remote tool server address cannot target cloud metadata or link-local networks")
	ErrNoCommand        = errors.New("admin: an MCP server needs a command to run")
	ErrBadMCPProtocol   = errors.New("admin: unknown MCP protocol mode")
	ErrBadConfigFileEnv = errors.New(
		"admin: the managed config file environment variable must be a shell variable name")
	ErrBadMCPRateLimit = errors.New(
		"admin: MCP rate limit needs both a positive rate and a positive burst, or neither")
	// ErrLocalExecutionNotAccepted means a local server was configured without
	// anybody saying they accept what a local server is.
	//
	// Refused on the way in and refused again when it is reached. A row can
	// arrive by restore or by a version of this that did not check, and the
	// runtime must not trust a rule it only enforces at the door.
	ErrLocalExecutionNotAccepted = errors.New(
		"admin: a local server runs code inside the worker, and that has to be accepted explicitly")
	ErrNoBaseURL         = errors.New("admin: a provider needs a base URL")
	ErrOAuthGrantChanged = errors.New(
		"admin: the stored OAuth grant changed before the refresh could be saved")
	ErrMCPServerUnknown           = errors.New("admin: no such MCP server")
	ErrMCPServerDisabled          = errors.New("admin: the MCP server is disabled")
	ErrMCPPersonalCredentialLocal = errors.New(
		"admin: personal MCP credentials only apply to remote HTTP servers")
	ErrNoMCPCredential              = errors.New("admin: no MCP credential was supplied")
	ErrMCPPersonalCredentialCorrupt = errors.New("admin: personal MCP credential metadata is corrupted")
)

const kindMCPProbe settings.Kind = "mcp_probe"

// The configured shapes are domain types: the API renders them and this
// package writes them, and neither imports the other. What lives here is only
// how they are encoded into a settings row.

type storedServer struct {
	Transport    string   `json:"transport,omitempty"`
	Command      string   `json:"command,omitempty"`
	Args         []string `json:"args,omitempty"`
	URL          string   `json:"url,omitempty"`
	ProtocolMode string   `json:"protocolMode,omitempty"`
	// ConfigFileEnv is metadata about a sealed config file, not the file
	// itself. Empty means the platform default.
	ConfigFileEnv string `json:"configFileEnv,omitempty"`
	// HasConfigFile mirrors the sealed document without exposing it, so a
	// read-model can offer the revoke gesture without opening the vault.
	HasConfigFile bool `json:"hasConfigFile,omitempty"`
	// HasVariables is the same read-model hint for local process variables.
	HasVariables bool `json:"hasVariables,omitempty"`
	// HasOAuth is the same read-model hint for a remote OAuth grant.
	HasOAuth bool `json:"hasOAuth,omitempty"`
	/*
		Surface is which of the server's tools this installation brought in.

		A pointer, because absent and empty are different answers and the
		difference is an upgrade. A server nobody has chosen a surface for goes
		on offering what it always did; one chosen as empty offers nothing.
	*/
	Surface *[]string `json:"surface,omitempty"`
	// RateLimit is per worker process. In a deployment with multiple workers,
	// the effective cap scales with the replica count; it is an operational
	// guard, not a distributed quota.
	RateLimit *domain.MCPRateLimit `json:"rateLimit,omitempty"`
	// AcceptsLocalExecution is stored rather than assumed from the transport.
	// A row written before this existed has not been accepted by anybody, and
	// reading it as accepted would grant on upgrade what nobody granted.
	AcceptsLocalExecution bool `json:"acceptsLocalExecution,omitempty"`
}

type storedProvider struct {
	Kind    string `json:"kind"`
	BaseURL string `json:"baseURL"`
}

type storedMCPUserCredential struct {
	Server     string `json:"server"`
	Principal  string `json:"principal"`
	HasHeaders bool   `json:"hasHeaders,omitempty"`
	HasOAuth   bool   `json:"hasOAuth,omitempty"`
}

// Integrations configures what the platform talks to.
type Integrations struct {
	pool     *pgxpool.Pool
	settings *settings.Store
	// health is where observations are forgotten when a server is removed.
	health *Health
}

func NewIntegrations(pool *pgxpool.Pool, store *settings.Store) *Integrations {
	return &Integrations{pool: pool, settings: store}
}

func (i *Integrations) MCPServers(ctx context.Context) ([]domain.MCPServer, error) {
	rows, err := i.settings.List(ctx, settings.KindMCPServer)
	if err != nil {
		return nil, err
	}

	out := make([]domain.MCPServer, 0, len(rows))
	for _, row := range rows {
		var stored storedServer
		if err := json.Unmarshal(row.Value, &stored); err != nil {
			return nil, fmt.Errorf("admin: decode MCP server %s: %w", row.Name, err)
		}
		transport := stored.Transport
		if transport == "" {
			transport = domain.TransportStdio
		}
		hasVariables := stored.HasVariables
		if !hasVariables && row.HasSecret && transport == domain.TransportStdio && !stored.HasConfigFile {
			// Rows written after per-server env existed but before this
			// read-model bit have a sealed document and no way to say which
			// half. Config files did not exist then, so keep the old revoke
			// affordance for local variables instead of hiding it on upgrade.
			hasVariables = true
		}
		server := domain.MCPServer{
			Name: row.Name, Transport: stored.Transport,
			Command: stored.Command, Args: stored.Args, URL: stored.URL,
			ProtocolMode:          stored.ProtocolMode,
			Surface:               stored.Surface,
			RateLimit:             stored.RateLimit,
			AcceptsLocalExecution: stored.AcceptsLocalExecution,
			HasSecret:             row.HasSecret,
			HasOAuth:              stored.HasOAuth,
			HasVariables:          hasVariables,
			HasConfigFile:         stored.HasConfigFile,
			ConfigFileEnv:         configFileEnv(stored.ConfigFileEnv),
			Enabled:               row.Enabled,
			UpdatedBy:             row.UpdatedBy, UpdatedAt: row.UpdatedAt,
		}
		server.Transport = server.TransportOf()
		out = append(out, server)
	}
	return out, nil
}

/*
PutMCPServer records a server and who configured it.

A field the request does not mention is left as it is: a token nobody
re-entered, a surface this write was not about. What removes something is
sending it empty, which is a different request from not sending it — and both
folds happen inside the write's transaction, so "as it is" means as it is when
the write happens rather than when the request was read.
*/
func (i *Integrations) PutMCPServer(
	ctx context.Context, by domain.UserID, scope domain.Scope,
	server domain.MCPServer, given domain.MCPCredentialPatch,
) error {
	transport := server.TransportOf()
	switch {
	case strings.TrimSpace(server.Name) == "":
		return ErrNoName
	case transport == domain.TransportStdio && strings.TrimSpace(server.Command) == "":
		return ErrNoCommand
	case transport == domain.TransportStdio && !server.AcceptsLocalExecution:
		return ErrLocalExecutionNotAccepted
	case transport == domain.TransportHTTP && strings.TrimSpace(server.URL) == "":
		return ErrNoURL
	case transport == domain.TransportHTTP:
		if err := netguard.ValidateHTTPURL(server.URL); errors.Is(err, netguard.ErrBlockedAddress) {
			return fmt.Errorf("%w: %v", ErrBlockedMCPURL, err)
		} else if err != nil {
			return fmt.Errorf("%w: %v", ErrBadMCPURL, err)
		}
	case server.MCPProtocolModeOf() != domain.MCPProtocolAuto &&
		server.MCPProtocolModeOf() != domain.MCPProtocolLegacy:
		return ErrBadMCPProtocol
	case transport != domain.TransportHTTP && server.MCPProtocolModeOf() != domain.MCPProtocolAuto:
		return ErrBadMCPProtocol
	case server.ConfigFileEnv != nil && !validConfigFileEnv(*server.ConfigFileEnv):
		return ErrBadConfigFileEnv
	}
	rateLimit, err := normalizedRateLimit(server.RateLimit)
	if err != nil {
		return err
	}

	/*
		Both folds happen inside the write's own transaction, under the row's
		lock.

		Read before it, either one is a lost update waiting for two people: one
		narrows the server's surface, the other saves a token having read the
		older value, and whichever commits second puts the older value back.
		The dangerous direction is the same for both — a surface restored to
		"nobody chose" reopens every tool, and a credential restored is one
		somebody believes they revoked.
	*/
	return writeFolded(ctx, i.pool, i.settings, folded{
		by: by, scope: scope, action: "mcp_server.configured", target: server.Name,
		set: settings.Setting{
			ScopeKind: settings.ScopeInstallation,
			Kind:      settings.KindMCPServer, Name: server.Name,
			Enabled: server.Enabled, UpdatedBy: string(by),
		},
		fold: func(stored settings.Setting) (settings.Setting, any, error) {
			var was storedServer
			if len(stored.Value) > 0 {
				if err := json.Unmarshal(stored.Value, &was); err != nil {
					return settings.Setting{}, nil, fmt.Errorf(
						"admin: read the stored server for %s: %w", server.Name, err)
				}
			}

			surface := server.Surface
			if surface == nil {
				surface = was.Surface
			}
			if server.RateLimit == nil {
				rateLimit = was.RateLimit
			}
			configEnv := was.ConfigFileEnv
			if server.ConfigFileEnv != nil {
				configEnv = *server.ConfigFileEnv
			}

			merged := given.Apply(domain.ReadMCPCredentials(stored.Secret)).ForTransport(transport)
			if transport != domain.TransportStdio {
				configEnv = ""
			}
			value, err := json.Marshal(storedServer{
				Transport: transport, Command: server.Command,
				Args: server.Args, URL: server.URL,
				ProtocolMode:          storedProtocolMode(transport, server.MCPProtocolModeOf()),
				Surface:               surface,
				RateLimit:             rateLimit,
				ConfigFileEnv:         configEnv,
				HasVariables:          len(merged.Env) > 0,
				HasOAuth:              merged.OAuth != nil && !merged.OAuth.Empty(),
				HasConfigFile:         merged.ConfigFile != "",
				AcceptsLocalExecution: server.AcceptsLocalExecution,
			})
			if err != nil {
				return settings.Setting{}, nil, fmt.Errorf("admin: encode MCP server: %w", err)
			}

			return settings.Setting{
					ScopeKind: settings.ScopeInstallation,
					Kind:      settings.KindMCPServer, Name: server.Name,
					Value: value, Secret: merged.Sealed(),
					// Nothing left to keep is a removal to carry out, not a
					// write to skip: the store's own rule makes "clear it" and
					// "do not mention it" the same request otherwise, and one
					// of those is somebody revoking.
					ClearSecret: merged.Empty(),
					Enabled:     server.Enabled, UpdatedBy: string(by),
				}, map[string]any{
					// Never a credential, only which of them are now held.
					//
					// `transport`, not `kind`: `kind` is the provider's word,
					// and an administrative trail that renames a field is a
					// trail whose older half no longer answers the query that
					// reads its newer half.
					"transport": transport, "command": server.Command, "url": server.URL,
					"protocolMode": server.MCPProtocolModeOf(),
					"enabled":      server.Enabled, "tokenChanged": given.Token != nil,
					"headersChanged":        given.Headers != nil,
					"oauthChanged":          given.OAuth != nil,
					"acceptsLocalExecution": server.AcceptsLocalExecution,
					"variables":             len(merged.Env),
					"hasOAuth":              merged.OAuth != nil && !merged.OAuth.Empty(),
					"configFileChanged":     given.ConfigFile != nil,
					"hasConfigFile":         merged.ConfigFile != "",
					"configFileEnv":         configEnv,
					"surface":               surfaceSize(surface),
					"rateLimit":             rateLimitDetail(rateLimit),
				}, nil
		},
	})
}

func configFileEnv(name string) *string {
	if name == "" {
		return nil
	}
	return &name
}

func storedProtocolMode(transport, mode string) string {
	if transport != domain.TransportHTTP || mode == domain.MCPProtocolAuto {
		return ""
	}
	return mode
}

func normalizedRateLimit(limit *domain.MCPRateLimit) (*domain.MCPRateLimit, error) {
	if limit == nil {
		return nil, nil
	}
	disabled := limit.RatePerSecond == 0 && limit.Burst == 0
	if disabled {
		return nil, nil
	}
	if math.IsNaN(limit.RatePerSecond) || math.IsInf(limit.RatePerSecond, 0) ||
		limit.RatePerSecond <= 0 || limit.Burst <= 0 {
		return nil, ErrBadMCPRateLimit
	}
	return &domain.MCPRateLimit{RatePerSecond: limit.RatePerSecond, Burst: limit.Burst}, nil
}

func rateLimitDetail(limit *domain.MCPRateLimit) any {
	if limit == nil {
		return nil
	}
	return map[string]any{"ratePerSecond": limit.RatePerSecond, "burst": limit.Burst}
}

var configFileEnvPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validConfigFileEnv(name string) bool {
	return name == "" || configFileEnvPattern.MatchString(name)
}

// MCPCredentials opens what a server was given. Separate and explicit:
// reading configuration is routine, reading a credential is not.
func (i *Integrations) MCPCredentials(
	ctx context.Context, name string,
) (domain.MCPCredentials, error) {
	set, err := i.settings.Reveal(ctx, settings.ScopeInstallation, domain.Scope{}, settings.KindMCPServer, name)
	if err != nil {
		return domain.MCPCredentials{}, err
	}
	return domain.ReadMCPCredentials(set.Secret), nil
}

// MCPPersonalCredentials lists the caller's own remote MCP credentials,
// without exposing the material sealed under them.
func (i *Integrations) MCPPersonalCredentials(
	ctx context.Context, principal domain.UserID,
) ([]domain.MCPPersonalCredential, error) {
	rows, err := i.settings.List(ctx, settings.KindMCPUserCredential)
	if err != nil {
		return nil, err
	}

	out := make([]domain.MCPPersonalCredential, 0, len(rows))
	for _, row := range rows {
		var stored storedMCPUserCredential
		if err := json.Unmarshal(row.Value, &stored); err != nil {
			return nil, fmt.Errorf("admin: decode personal MCP credential %s: %w", row.Name, err)
		}
		if stored.Principal != string(principal) {
			continue
		}
		out = append(out, domain.MCPPersonalCredential{
			Server: rowServer(stored, row.Name), Principal: principal,
			HasSecret: row.HasSecret, HasHeaders: stored.HasHeaders,
			HasOAuth:  stored.HasOAuth,
			UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
		})
	}
	slices.SortFunc(out, func(a, b domain.MCPPersonalCredential) int {
		return strings.Compare(a.Server, b.Server)
	})
	return out, nil
}

// MCPPersonalCredential opens one person's credential for one server. It is
// used only at call time, when the run has an OnBehalfOf to name whose
// credential may be sent.
func (i *Integrations) MCPPersonalCredential(
	ctx context.Context, name string, principal domain.UserID,
) (domain.MCPCredentials, bool, error) {
	if principal == "" {
		return domain.MCPCredentials{}, false, nil
	}
	set, err := i.settings.Reveal(ctx,
		settings.ScopeInstallation, domain.Scope{},
		settings.KindMCPUserCredential, mcpUserCredentialName(name, principal))
	switch {
	case errors.Is(err, settings.ErrNotFound):
		return domain.MCPCredentials{}, false, nil
	case err != nil:
		return domain.MCPCredentials{}, false, err
	case !set.HasSecret:
		return domain.MCPCredentials{}, false, nil
	}
	if _, err := storedPersonalCredential(set, name, principal); err != nil {
		return domain.MCPCredentials{}, false, err
	}
	return domain.ReadMCPCredentials(set.Secret).ForTransport(domain.TransportHTTP), true, nil
}

// PutMCPPersonalCredential stores the caller's own credential for a configured
// remote server.
func (i *Integrations) PutMCPPersonalCredential(
	ctx context.Context, by domain.UserID, scope domain.Scope,
	name string, given domain.MCPCredentialPatch,
) error {
	if strings.TrimSpace(name) == "" {
		return ErrNoName
	}
	if err := i.personalCredentialTarget(ctx, name); err != nil {
		return err
	}

	key := mcpUserCredentialName(name, by)
	return writeFolded(ctx, i.pool, i.settings, folded{
		by: by, scope: scope, action: "mcp_credential.configured", target: name,
		set: settings.Setting{
			ScopeKind: settings.ScopeInstallation,
			Kind:      settings.KindMCPUserCredential, Name: key,
			Enabled: true, UpdatedBy: string(by),
		},
		fold: func(stored settings.Setting) (settings.Setting, any, error) {
			merged := given.Apply(domain.ReadMCPCredentials(stored.Secret)).
				ForTransport(domain.TransportHTTP)
			if merged.Empty() {
				return settings.Setting{}, nil, ErrNoMCPCredential
			}
			value, err := json.Marshal(storedMCPUserCredential{
				Server: name, Principal: string(by),
				HasHeaders: len(merged.Headers) > 0,
				HasOAuth:   merged.OAuth != nil && !merged.OAuth.Empty(),
			})
			if err != nil {
				return settings.Setting{}, nil, fmt.Errorf(
					"admin: encode personal MCP credential: %w", err)
			}
			return settings.Setting{
					ScopeKind: settings.ScopeInstallation,
					Kind:      settings.KindMCPUserCredential, Name: key,
					Value: value, Secret: merged.Sealed(),
					Enabled: true, UpdatedBy: string(by),
				}, map[string]any{
					"server": name, "principal": string(by),
					"tokenChanged":   given.Token != nil,
					"headersChanged": given.Headers != nil,
					"oauthChanged":   given.OAuth != nil,
					"hasHeaders":     len(merged.Headers) > 0,
					"hasOAuth":       merged.OAuth != nil && !merged.OAuth.Empty(),
				}, nil
		},
	})
}

func (i *Integrations) DeleteMCPPersonalCredential(
	ctx context.Context, by domain.UserID, scope domain.Scope, name string,
) error {
	if strings.TrimSpace(name) == "" {
		return ErrNoName
	}
	key := mcpUserCredentialName(name, by)
	detail, err := json.Marshal(map[string]any{
		"server": name, "principal": string(by),
	})
	if err != nil {
		return fmt.Errorf("admin: encode personal MCP credential removal: %w", err)
	}

	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := i.settings.DeleteTx(ctx, tx,
		settings.ScopeInstallation, domain.Scope{},
		settings.KindMCPUserCredential, key); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: "mcp_credential.removed",
		Target: name, Detail: detail,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (i *Integrations) personalCredentialTarget(ctx context.Context, name string) error {
	set, err := i.settings.Get(ctx,
		settings.ScopeInstallation, domain.Scope{}, settings.KindMCPServer, name)
	switch {
	case errors.Is(err, settings.ErrNotFound):
		return ErrMCPServerUnknown
	case err != nil:
		return err
	}
	var stored storedServer
	if len(set.Value) > 0 {
		if err := json.Unmarshal(set.Value, &stored); err != nil {
			return fmt.Errorf("admin: decode MCP server %s: %w", name, err)
		}
	}
	transport := stored.Transport
	if transport == "" {
		transport = domain.TransportStdio
	}
	if transport != domain.TransportHTTP {
		return ErrMCPPersonalCredentialLocal
	}
	return nil
}

func mcpUserCredentialName(server string, principal domain.UserID) string {
	return encodeSettingPart(server) + "." + encodeSettingPart(string(principal))
}

func storedPersonalCredential(
	set settings.Setting, server string, principal domain.UserID,
) (storedMCPUserCredential, error) {
	var stored storedMCPUserCredential
	if err := json.Unmarshal(set.Value, &stored); err != nil {
		return storedMCPUserCredential{}, fmt.Errorf("%w: %s: %v",
			ErrMCPPersonalCredentialCorrupt, set.Name, err)
	}
	if stored.Principal != string(principal) || rowServer(stored, set.Name) != server {
		return storedMCPUserCredential{}, fmt.Errorf("%w: %s",
			ErrMCPPersonalCredentialCorrupt, set.Name)
	}
	return stored, nil
}

func encodeSettingPart(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func rowServer(stored storedMCPUserCredential, name string) string {
	if stored.Server != "" {
		return stored.Server
	}
	server, _, ok := strings.Cut(name, ".")
	if !ok {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(server)
	if err != nil {
		return ""
	}
	return string(raw)
}

// RequestMCPProbe records that an operator asked a worker to reach a server.
//
// The API process must not do this itself. For stdio it would start code in
// the API pod, not in the worker that will later offer the tools to agents.
// The request is therefore a small durable queue item: the worker consumes it
// and runs the same connection path ordinary reconciliation uses.
func (i *Integrations) RequestMCPProbe(
	ctx context.Context, by domain.UserID, scope domain.Scope, name string,
) error {
	if strings.TrimSpace(name) == "" {
		return ErrNoName
	}
	server, err := i.settings.Get(ctx,
		settings.ScopeInstallation, domain.Scope{}, settings.KindMCPServer, name)
	switch {
	case errors.Is(err, settings.ErrNotFound):
		return ErrMCPServerUnknown
	case err != nil:
		return err
	case !server.Enabled:
		return ErrMCPServerDisabled
	}

	return writeSetting(ctx, i.pool, i.settings, by, scope, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      kindMCPProbe,
		Name:      name,
		Value:     json.RawMessage(`{}`),
		Enabled:   true,
		UpdatedBy: string(by),
	}, "mcp_server.probe_requested", name, map[string]any{"server": name})
}

// ClaimMCPProbes returns server names whose explicit probe requests this worker
// owns now, removing the request rows in the same statement.
func (i *Integrations) ClaimMCPProbes(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := i.pool.Query(ctx, `
		with picked as materialized (
			select scope_kind, company_id, area_id, kind, name
			from settings
			where scope_kind = $1 and company_id = '' and area_id = '' and kind = $2
			order by updated_at
			for update skip locked
			limit $3
		)
		delete from settings s
		using picked p
		where s.scope_kind = p.scope_kind
		  and s.company_id = p.company_id
		  and s.area_id = p.area_id
		  and s.kind = p.kind
		  and s.name = p.name
		returning s.name`,
		string(settings.ScopeInstallation), string(kindMCPProbe), limit)
	if err != nil {
		return nil, fmt.Errorf("admin: claim MCP probes: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

/*
RefreshMCPServerOAuth persists the grant a worker received from an OAuth
refresh.

It is deliberately not PutMCPServer with a different caller. Refresh is the
worker preserving an operator's earlier credential decision, not a new
configuration act by a person. It still takes the one configuration write path:
the stored secret and the event that explains it commit together.
*/
func (i *Integrations) RefreshMCPServerOAuth(
	ctx context.Context, name string, principal domain.UserID, was, next domain.MCPOAuthGrant,
) error {
	if strings.TrimSpace(name) == "" {
		return ErrNoName
	}
	if next.Empty() {
		return ErrOAuthGrantChanged
	}
	if principal != "" {
		return i.refreshMCPPersonalOAuth(ctx, name, principal, was, next)
	}
	return writeFolded(ctx, i.pool, i.settings, folded{
		by:     domain.SystemWorker,
		scope:  domain.Scope{Company: domain.Installation},
		action: "mcp_oauth.refreshed",
		target: name,
		set: settings.Setting{
			ScopeKind: settings.ScopeInstallation,
			Kind:      settings.KindMCPServer, Name: name,
			UpdatedBy: string(domain.SystemWorker),
		},
		fold: func(stored settings.Setting) (settings.Setting, any, error) {
			if stored.Kind == "" {
				return settings.Setting{}, nil, ErrOAuthGrantChanged
			}
			var server storedServer
			if len(stored.Value) > 0 {
				if err := json.Unmarshal(stored.Value, &server); err != nil {
					return settings.Setting{}, nil, fmt.Errorf(
						"admin: read the stored server for %s: %w", name, err)
				}
			}
			transport := server.Transport
			if transport == "" {
				transport = domain.TransportStdio
			}
			if transport != domain.TransportHTTP {
				return settings.Setting{}, nil, ErrOAuthGrantChanged
			}

			creds := domain.ReadMCPCredentials(stored.Secret).ForTransport(domain.TransportHTTP)
			if creds.OAuth == nil || !creds.OAuth.Equal(was) {
				return settings.Setting{}, nil, ErrOAuthGrantChanged
			}
			merged := domain.MCPCredentialPatch{OAuth: &next}.Apply(creds).ForTransport(domain.TransportHTTP)
			server.HasOAuth = merged.OAuth != nil && !merged.OAuth.Empty()
			value, err := json.Marshal(server)
			if err != nil {
				return settings.Setting{}, nil, fmt.Errorf("admin: encode MCP server: %w", err)
			}

			return settings.Setting{
					ScopeKind: settings.ScopeInstallation,
					Kind:      settings.KindMCPServer, Name: name,
					Value: value, Secret: merged.Sealed(),
					ClearSecret: merged.Empty(),
					Enabled:     stored.Enabled, UpdatedBy: string(domain.SystemWorker),
				}, map[string]any{
					"accessTokenChanged":  was.AccessToken != next.AccessToken,
					"refreshTokenChanged": was.RefreshToken != next.RefreshToken,
					"tokenTypeChanged":    was.TokenType != next.TokenType,
					"expiresAtChanged":    was.ExpiresAtUnix != next.ExpiresAtUnix,
					"scopesChanged":       !slices.Equal(was.Scopes, next.Scopes),
					"hasRefreshToken":     next.RefreshToken != "",
					"scopeCount":          len(next.Scopes),
				}, nil
		},
	})
}

func (i *Integrations) refreshMCPPersonalOAuth(
	ctx context.Context, name string, principal domain.UserID,
	was, next domain.MCPOAuthGrant,
) error {
	key := mcpUserCredentialName(name, principal)
	return writeFolded(ctx, i.pool, i.settings, folded{
		by:     domain.SystemWorker,
		scope:  domain.Scope{Company: domain.Installation},
		action: "mcp_personal_oauth.refreshed",
		target: name,
		set: settings.Setting{
			ScopeKind: settings.ScopeInstallation,
			Kind:      settings.KindMCPUserCredential, Name: key,
			UpdatedBy: string(domain.SystemWorker),
		},
		fold: func(stored settings.Setting) (settings.Setting, any, error) {
			if stored.Kind == "" {
				return settings.Setting{}, nil, ErrOAuthGrantChanged
			}
			if _, err := storedPersonalCredential(stored, name, principal); err != nil {
				return settings.Setting{}, nil, err
			}
			creds := domain.ReadMCPCredentials(stored.Secret).ForTransport(domain.TransportHTTP)
			if creds.OAuth == nil || !creds.OAuth.Equal(was) {
				return settings.Setting{}, nil, ErrOAuthGrantChanged
			}
			merged := domain.MCPCredentialPatch{OAuth: &next}.Apply(creds).ForTransport(domain.TransportHTTP)
			value, err := json.Marshal(storedMCPUserCredential{
				Server: name, Principal: string(principal),
				HasHeaders: len(merged.Headers) > 0,
				HasOAuth:   merged.OAuth != nil && !merged.OAuth.Empty(),
			})
			if err != nil {
				return settings.Setting{}, nil, fmt.Errorf(
					"admin: encode personal MCP credential: %w", err)
			}

			return settings.Setting{
					ScopeKind: settings.ScopeInstallation,
					Kind:      settings.KindMCPUserCredential, Name: key,
					Value: value, Secret: merged.Sealed(),
					ClearSecret: merged.Empty(),
					Enabled:     stored.Enabled, UpdatedBy: string(domain.SystemWorker),
				}, map[string]any{
					"server": name, "principal": string(principal),
					"accessTokenChanged":  was.AccessToken != next.AccessToken,
					"refreshTokenChanged": was.RefreshToken != next.RefreshToken,
					"tokenTypeChanged":    was.TokenType != next.TokenType,
					"expiresAtChanged":    was.ExpiresAtUnix != next.ExpiresAtUnix,
					"scopesChanged":       !slices.Equal(was.Scopes, next.Scopes),
					"hasRefreshToken":     next.RefreshToken != "",
					"scopeCount":          len(next.Scopes),
				}, nil
		},
	})
}

// ForgettingHealth wires where observations are dropped when a server is
// removed. Optional: a store without it still removes the configuration.
func (i *Integrations) ForgettingHealth(h *Health) *Integrations {
	i.health = h
	return i
}

func (i *Integrations) DeleteMCPServer(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error {
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := i.settings.DeleteTx(ctx, tx,
		settings.ScopeInstallation, domain.Scope{},
		settings.KindMCPServer, name); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		delete from settings
		where scope_kind = $1 and company_id = '' and area_id = ''
		  and kind = $2 and (value->>'server' = $3 or name like $4)`,
		string(settings.ScopeInstallation), string(settings.KindMCPUserCredential), name,
		encodeSettingPart(name)+".%")
	if err != nil {
		return fmt.Errorf("admin: delete personal MCP credentials: %w", err)
	}
	detail, err := json.Marshal(map[string]any{
		"personalCredentialsRemoved": tag.RowsAffected(),
	})
	if err != nil {
		return fmt.Errorf("admin: encode MCP server removal: %w", err)
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: "mcp_server.removed", Target: name,
		Detail: detail,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if i.health == nil {
		return nil
	}
	// After the configuration, and separately: the removal has happened either
	// way, and a screen showing a stale observation is better than a server
	// that could not be removed because its observation could not be.
	return i.health.Forget(ctx, name)
}

func (i *Integrations) Providers(ctx context.Context) ([]domain.ModelProvider, error) {
	rows, err := i.settings.List(ctx, settings.KindModelProvider)
	if err != nil {
		return nil, err
	}

	out := make([]domain.ModelProvider, 0, len(rows))
	for _, row := range rows {
		var stored storedProvider
		if err := json.Unmarshal(row.Value, &stored); err != nil {
			return nil, fmt.Errorf("admin: decode provider %s: %w", row.Name, err)
		}
		out = append(out, domain.ModelProvider{
			Name: row.Name, Kind: stored.Kind, BaseURL: stored.BaseURL,
			Enabled: row.Enabled, HasKey: row.HasSecret,
			UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

// PutProvider records a provider. An empty key keeps the stored one, so
// changing a base URL does not require re-entering a credential — which is how
// operators end up pasting keys into chat to look them up.
func (i *Integrations) PutProvider(ctx context.Context, by domain.UserID, scope domain.Scope, provider domain.ModelProvider, apiKey string) error {
	switch {
	case strings.TrimSpace(provider.Name) == "":
		return ErrNoName
	// Required only where the client cannot already know it. Anthropic's does;
	// an OpenAI-compatible endpoint, including a self-hosted one, is known
	// only to the installation. Demanding it from everybody asked for a value
	// nobody has and made the reference provider impossible to configure.
	case provider.Kind != "anthropic" && strings.TrimSpace(provider.BaseURL) == "":
		return ErrNoBaseURL
	}

	value, err := json.Marshal(storedProvider{Kind: provider.Kind, BaseURL: provider.BaseURL})
	if err != nil {
		return fmt.Errorf("admin: encode provider: %w", err)
	}

	return writeSetting(ctx, i.pool, i.settings, by, scope, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      settings.KindModelProvider,
		Name:      provider.Name,
		Value:     value,
		Secret:    apiKey,
		Enabled:   provider.Enabled,
		UpdatedBy: string(by),
	}, "provider.configured", provider.Name, map[string]any{
		// The credential is never in the trail, only the fact that one arrived.
		"kind": provider.Kind, "baseURL": provider.BaseURL,
		"enabled": provider.Enabled, "keyChanged": apiKey != "",
	})
}

func (i *Integrations) DeleteProvider(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error {
	return removeSetting(ctx, i.pool, i.settings, by, scope, settings.KindModelProvider, name, "provider.removed")
}

// Credential opens a provider's key. Separate and explicit: reading
// configuration is routine, reading a credential is not.
func (i *Integrations) Credential(ctx context.Context, name string) (string, error) {
	set, err := i.settings.Reveal(ctx, settings.ScopeInstallation, domain.Scope{}, settings.KindModelProvider, name)
	if err != nil {
		return "", err
	}
	return set.Secret, nil
}

// surfaceSize reports how many tools were brought in, or -1 for a server
// nobody has chosen a surface for. The two are different facts and an
// administrative record that showed both as zero would be recording the
// opposite of what happened for one of them.
func surfaceSize(surface *[]string) int {
	if surface == nil {
		return -1
	}
	return len(*surface)
}
