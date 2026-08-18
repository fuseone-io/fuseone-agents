package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

var (
	ErrNoName           = errors.New("admin: an integration needs a name")
	ErrNoURL            = errors.New("admin: a remote tool server needs an address")
	ErrNoCommand        = errors.New("admin: an MCP server needs a command to run")
	ErrBadConfigFileEnv = errors.New(
		"admin: the managed config file environment variable must be a shell variable name")
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
)

// The configured shapes are domain types: the API renders them and this
// package writes them, and neither imports the other. What lives here is only
// how they are encoded into a settings row.

type storedServer struct {
	Transport string   `json:"transport,omitempty"`
	Command   string   `json:"command,omitempty"`
	Args      []string `json:"args,omitempty"`
	URL       string   `json:"url,omitempty"`
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
	// AcceptsLocalExecution is stored rather than assumed from the transport.
	// A row written before this existed has not been accepted by anybody, and
	// reading it as accepted would grant on upgrade what nobody granted.
	AcceptsLocalExecution bool `json:"acceptsLocalExecution,omitempty"`
}

type storedProvider struct {
	Kind    string `json:"kind"`
	BaseURL string `json:"baseURL"`
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
			Surface:               stored.Surface,
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
	case server.ConfigFileEnv != nil && !validConfigFileEnv(*server.ConfigFileEnv):
		return ErrBadConfigFileEnv
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
				Args: server.Args, URL: server.URL, Surface: surface,
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
					"enabled": server.Enabled, "tokenChanged": given.Token != nil,
					"oauthChanged":          given.OAuth != nil,
					"acceptsLocalExecution": server.AcceptsLocalExecution,
					"variables":             len(merged.Env),
					"hasOAuth":              merged.OAuth != nil && !merged.OAuth.Empty(),
					"configFileChanged":     given.ConfigFile != nil,
					"hasConfigFile":         merged.ConfigFile != "",
					"configFileEnv":         configEnv,
					"surface":               surfaceSize(surface),
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

/*
RefreshMCPServerOAuth persists the grant a worker received from an OAuth
refresh.

It is deliberately not PutMCPServer with a different caller. Refresh is the
worker preserving an operator's earlier credential decision, not a new
configuration act by a person. It still takes the one configuration write path:
the stored secret and the event that explains it commit together.
*/
func (i *Integrations) RefreshMCPServerOAuth(
	ctx context.Context, name string, was, next domain.MCPOAuthGrant,
) error {
	if strings.TrimSpace(name) == "" {
		return ErrNoName
	}
	if next.Empty() {
		return ErrOAuthGrantChanged
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

// ForgettingHealth wires where observations are dropped when a server is
// removed. Optional: a store without it still removes the configuration.
func (i *Integrations) ForgettingHealth(h *Health) *Integrations {
	i.health = h
	return i
}

func (i *Integrations) DeleteMCPServer(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error {
	if err := removeSetting(ctx, i.pool, i.settings, by, scope,
		settings.KindMCPServer, name, "mcp_server.removed"); err != nil {
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
