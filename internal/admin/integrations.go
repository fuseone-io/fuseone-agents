package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

var (
	ErrNoName    = errors.New("admin: an integration needs a name")
	ErrNoURL     = errors.New("admin: a remote tool server needs an address")
	ErrNoCommand = errors.New("admin: an MCP server needs a command to run")
	ErrNoBaseURL = errors.New("admin: a provider needs a base URL")
)

// The configured shapes are domain types: the API renders them and this
// package writes them, and neither imports the other. What lives here is only
// how they are encoded into a settings row.

type storedServer struct {
	Transport string   `json:"transport,omitempty"`
	Command   string   `json:"command,omitempty"`
	Args      []string `json:"args,omitempty"`
	URL       string   `json:"url,omitempty"`
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
		server := domain.MCPServer{
			Name: row.Name, Transport: stored.Transport,
			Command: stored.Command, Args: stored.Args, URL: stored.URL,
			HasSecret: row.HasSecret, Enabled: row.Enabled,
			UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
		}
		server.Transport = server.TransportOf()
		out = append(out, server)
	}
	return out, nil
}

// PutMCPServer records a server and who configured it.
//
// An empty token keeps whichever one is stored, like every other credential
// here: correcting a URL must not demand re-entering a secret nobody has to
// hand.
func (i *Integrations) PutMCPServer(
	ctx context.Context, by domain.UserID, scope domain.Scope,
	server domain.MCPServer, token string,
) error {
	transport := server.TransportOf()
	switch {
	case strings.TrimSpace(server.Name) == "":
		return ErrNoName
	case transport == domain.TransportStdio && strings.TrimSpace(server.Command) == "":
		return ErrNoCommand
	case transport == domain.TransportHTTP && strings.TrimSpace(server.URL) == "":
		return ErrNoURL
	}

	value, err := json.Marshal(storedServer{
		Transport: transport, Command: server.Command, Args: server.Args, URL: server.URL,
	})
	if err != nil {
		return fmt.Errorf("admin: encode MCP server: %w", err)
	}

	return writeSetting(ctx, i.pool, i.settings, by, scope, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      settings.KindMCPServer,
		Name:      server.Name,
		Value:     value,
		Secret:    token,
		Enabled:   server.Enabled,
		UpdatedBy: string(by),
	}, "mcp_server.configured", server.Name, map[string]any{
		// Never the token, only that one arrived.
		"transport": transport, "command": server.Command, "url": server.URL,
		"enabled": server.Enabled, "tokenChanged": token != "",
	})
}

// MCPToken opens a remote server's bearer token. Separate and explicit:
// reading configuration is routine, reading a credential is not.
func (i *Integrations) MCPToken(ctx context.Context, name string) (string, error) {
	set, err := i.settings.Reveal(ctx, settings.ScopeInstallation, domain.Scope{}, settings.KindMCPServer, name)
	if err != nil {
		return "", err
	}
	return set.Secret, nil
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
