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
	ErrNoCommand = errors.New("admin: an MCP server needs a command to run")
	ErrNoBaseURL = errors.New("admin: a provider needs a base URL")
)

// The configured shapes are domain types: the API renders them and this
// package writes them, and neither imports the other. What lives here is only
// how they are encoded into a settings row.

type storedServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

type storedProvider struct {
	Kind    string `json:"kind"`
	BaseURL string `json:"baseURL"`
}

// Integrations configures what the platform talks to.
type Integrations struct {
	pool     *pgxpool.Pool
	settings *settings.Store
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
		out = append(out, domain.MCPServer{
			Name: row.Name, Command: stored.Command, Args: stored.Args,
			Enabled: row.Enabled, UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

// PutMCPServer records a server and who configured it.
func (i *Integrations) PutMCPServer(ctx context.Context, by domain.UserID, scope domain.Scope, server domain.MCPServer) error {
	switch {
	case strings.TrimSpace(server.Name) == "":
		return ErrNoName
	case strings.TrimSpace(server.Command) == "":
		return ErrNoCommand
	}

	value, err := json.Marshal(storedServer{Command: server.Command, Args: server.Args})
	if err != nil {
		return fmt.Errorf("admin: encode MCP server: %w", err)
	}

	return i.write(ctx, by, scope, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      settings.KindMCPServer,
		Name:      server.Name,
		Value:     value,
		Enabled:   server.Enabled,
		UpdatedBy: string(by),
	}, "mcp_server.configured", server.Name, map[string]any{
		"command": server.Command, "enabled": server.Enabled,
	})
}

func (i *Integrations) DeleteMCPServer(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error {
	return i.remove(ctx, by, scope, settings.KindMCPServer, name, "mcp_server.removed")
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
	case strings.TrimSpace(provider.BaseURL) == "":
		return ErrNoBaseURL
	}

	value, err := json.Marshal(storedProvider{Kind: provider.Kind, BaseURL: provider.BaseURL})
	if err != nil {
		return fmt.Errorf("admin: encode provider: %w", err)
	}

	return i.write(ctx, by, scope, settings.Setting{
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
	return i.remove(ctx, by, scope, settings.KindModelProvider, name, "provider.removed")
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

func (i *Integrations) write(
	ctx context.Context, by domain.UserID, scope domain.Scope,
	set settings.Setting, action, target string, detail any,
) error {
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := i.settings.PutTx(ctx, tx, set); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: action, Target: target, Detail: detail,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (i *Integrations) remove(
	ctx context.Context, by domain.UserID, scope domain.Scope,
	kind settings.Kind, name, action string,
) error {
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := i.settings.DeleteTx(ctx, tx, settings.ScopeInstallation, domain.Scope{}, kind, name); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: action, Target: name,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
