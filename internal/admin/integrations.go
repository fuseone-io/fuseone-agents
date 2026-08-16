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
	// ErrLocalExecutionNotAccepted means a local server was configured without
	// anybody saying they accept what a local server is.
	//
	// Refused on the way in and refused again when it is reached. A row can
	// arrive by restore or by a version of this that did not check, and the
	// runtime must not trust a rule it only enforces at the door.
	ErrLocalExecutionNotAccepted = errors.New(
		"admin: a local server runs code inside the worker, and that has to be accepted explicitly")
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
		server := domain.MCPServer{
			Name: row.Name, Transport: stored.Transport,
			Command: stored.Command, Args: stored.Args, URL: stored.URL,
			Surface:               stored.Surface,
			AcceptsLocalExecution: stored.AcceptsLocalExecution,
			HasSecret:             row.HasSecret, Enabled: row.Enabled,
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
	}

	/*
		A write that says nothing about the surface is not a request to forget
		it.

		Forgotten, it reads as "nobody has chosen", which the runtime treats as
		every tool the server offers — so saving a token or correcting a
		command would silently widen what agents can reach. That is the one
		direction this must never fail in, and it is reached by the two
		commonest edits there are.

		The same rule the credential already follows, for the same reason:
		absent is unchanged, and a chosen-and-empty surface stays chosen.
	*/
	surface, err := i.keptSurface(ctx, server)
	if err != nil {
		return err
	}

	value, err := json.Marshal(storedServer{
		Transport: transport, Command: server.Command, Args: server.Args, URL: server.URL,
		Surface:               surface,
		AcceptsLocalExecution: server.AcceptsLocalExecution,
	})
	if err != nil {
		return fmt.Errorf("admin: encode MCP server: %w", err)
	}

	/*
		Whichever half this write left out stays.

		Correcting an address must not demand re-entering a token nobody has to
		hand, and adding a variable must not drop one silently. The way to
		remove something is to send it empty, which is a different request from
		not sending it at all.
	*/
	merged, err := i.mergedCredentials(ctx, server.Name, given, transport)
	if err != nil {
		return err
	}

	return writeSetting(ctx, i.pool, i.settings, by, scope, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      settings.KindMCPServer,
		Name:      server.Name,
		Value:     value,
		Secret:    merged.Sealed(),
		// Nothing left to keep is a removal to carry out, not a write to skip.
		// Without saying so, the store's own rule — an omitted secret keeps
		// the stored one — makes "clear it" and "do not mention it" the same
		// request, and one of those is somebody taking a token back.
		ClearSecret: merged.Empty(),
		Enabled:     server.Enabled,
		UpdatedBy:   string(by),
	}, "mcp_server.configured", server.Name, map[string]any{
		// Never the token, only that one arrived.
		"transport": transport, "command": server.Command, "url": server.URL,
		"enabled": server.Enabled, "tokenChanged": given.Token != nil,
		// Which variables, never their values. That a server was given a
		// credential is a fact an auditor may need; the credential is not.
		"variables": len(merged.Env),
		// Who accepted local execution, and when, is the whole point of
		// recording it: the acceptance is a person's, not a checkbox's.
		"acceptsLocalExecution": server.AcceptsLocalExecution,
		// How many were brought in, never which: the list belongs on the
		// screen and the count is what an auditor reads as "this narrowed".
		"surface": surfaceSize(surface),
	})
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
mergedCredentials folds a write onto what is stored, and shapes it to the
transport.

Only "no such setting" reads as nothing stored. Every other failure — a vault
that will not open, a key that is wrong, a database that is away — used to read
the same way, so an edit made during any of them silently replaced a credential
the platform had simply failed to look at. A read that did not happen is not a
credential that does not exist.
*/
func (i *Integrations) mergedCredentials(
	ctx context.Context, name string, given domain.MCPCredentialPatch, transport string,
) (domain.MCPCredentials, error) {
	stored, err := i.MCPCredentials(ctx, name)
	switch {
	case errors.Is(err, settings.ErrNotFound):
		// The first write to a new server. Nothing stored is exactly what it
		// looks like here.
		stored = domain.MCPCredentials{}
	case err != nil:
		return domain.MCPCredentials{}, fmt.Errorf(
			"admin: read the stored credential for %s: %w", name, err)
	}
	return given.Apply(stored).ForTransport(transport), nil
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

// keptSurface answers what this write leaves the surface as. Only "no such
// setting" reads as nothing stored — every other failure is a read that did
// not happen, and treating one as an empty choice would open a server on the
// strength of a database being away.
func (i *Integrations) keptSurface(
	ctx context.Context, server domain.MCPServer,
) (*[]string, error) {
	if server.Surface != nil {
		return server.Surface, nil
	}
	stored, err := i.settings.Get(ctx,
		settings.ScopeInstallation, domain.Scope{}, settings.KindMCPServer, server.Name)
	switch {
	case errors.Is(err, settings.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("admin: read the stored surface for %s: %w", server.Name, err)
	}
	var was storedServer
	if err := json.Unmarshal(stored.Value, &was); err != nil {
		return nil, fmt.Errorf("admin: read the stored surface for %s: %w", server.Name, err)
	}
	return was.Surface, nil
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
