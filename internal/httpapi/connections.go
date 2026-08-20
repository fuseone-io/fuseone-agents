package httpapi

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/model"
)

/*
What this installation is connected to, and what it could be connected to.

The list is the connected servers and providers side by side with the ones
nobody has configured — an operator asking “can this agent reach the ERP?”
needs the absence answered as plainly as the presence.
*/
// Integrations is what the platform is configured to talk to, declared here by
// the consumer.
type Integrations interface {
	MCPServers(ctx context.Context) ([]domain.MCPServer, error)
	MCPPersonalCredentials(ctx context.Context, principal domain.UserID) ([]domain.MCPPersonalCredential, error)
	PutMCPPersonalCredential(ctx context.Context, by domain.UserID, scope domain.Scope, name string, creds domain.MCPCredentialPatch) error
	DeleteMCPPersonalCredential(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error
	Providers(ctx context.Context) ([]domain.ModelProvider, error)
	PutMCPServer(ctx context.Context, by domain.UserID, scope domain.Scope, server domain.MCPServer, creds domain.MCPCredentialPatch) error
	MCPCredentials(ctx context.Context, name string) (domain.MCPCredentials, error)
	RequestMCPProbe(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error
	DeleteMCPServer(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error
	PutProvider(ctx context.Context, by domain.UserID, scope domain.Scope, provider domain.ModelProvider, apiKey string) error
	DeleteProvider(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error
}

func healthFrom(seen domain.IntegrationHealth) openapi.IntegrationHealth {
	return openapi.IntegrationHealth{
		Reachable: seen.Reachable, ToolCount: seen.ToolCount,
		Detail: ptr(seen.Detail), ObservedAt: seen.ObservedAt,
		ObservedBy: ptr(seen.ObservedBy),
	}
}

func (s *Server) ListIntegrations(ctx context.Context, _ openapi.ListIntegrationsRequestObject) (openapi.ListIntegrationsResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermToolRead); resp != nil {
		return openapi.ListIntegrations403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}

	// The shapes the platform already knows how to reach, so the console can
	// fill in an endpoint instead of asking for one. An operator should have to
	// supply an address only when it is a proxy or a self-hosted model — the
	// two cases where the installation knows something the platform cannot.
	presets := knownProviders()
	body := openapi.ListIntegrations200JSONResponse{
		McpServers: []openapi.MCPServer{},
		Providers:  []openapi.ModelProvider{},
		Presets:    &presets,
	}
	if s.integrations == nil {
		return body, nil
	}

	servers, err := s.integrations.MCPServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list MCP servers: %w", err)
	}

	// What was observed, beside what was configured. A server can be enabled,
	// correct and unreachable, and only one of those three is somebody's
	// opinion.
	observed := map[string]domain.IntegrationHealth{}
	if s.health != nil {
		if observed, err = s.health.All(ctx); err != nil {
			return nil, fmt.Errorf("read integration health: %w", err)
		}
	}

	configured := map[string]bool{}
	for _, srv := range servers {
		configured[srv.Name] = true
		server := openapi.MCPServer{
			Name: srv.Name, Args: &srv.Args, Enabled: srv.Enabled,
			Transport:     ptr(openapi.Transport(srv.TransportOf())),
			ProtocolMode:  ptr(openapi.MCPProtocolMode(srv.MCPProtocolModeOf())),
			HasSecret:     ptr(srv.HasSecret),
			HasOAuth:      ptr(srv.HasOAuth),
			HasVariables:  ptr(srv.HasVariables),
			HasConfigFile: ptr(srv.HasConfigFile),
			Managed:       ptr(true),
			UpdatedBy:     ptr(srv.UpdatedBy), UpdatedAt: ptr(srv.UpdatedAt),
			// What was brought in, and who accepted running it. Both are
			// stored and neither reached the screen, so a server somebody had
			// narrowed read back as all-in — and saving anything from that
			// screen would have widened it back.
			//
			// The pointer is the whole point: nil is "nobody has chosen", and
			// an empty list is "chosen, and none of it".
			Surface:               srv.Surface,
			AcceptsLocalExecution: ptr(srv.AcceptsLocalExecution),
		}
		server.Command = someString(srv.Command)
		server.Url = someString(srv.URL)
		if srv.ConfigFileEnv != nil {
			server.ConfigFileEnv = someString(*srv.ConfigFileEnv)
		}
		// Absent when no worker has tried yet, which is a different thing from
		// a server that failed — and the screen has to be able to say so.
		if seen, tried := observed[srv.Name]; tried {
			server.Health = ptr(healthFrom(seen))
		}
		body.McpServers = append(body.McpServers, server)
	}

	// A server the platform is connected to but nobody configured here — passed
	// to the process by flag or environment. It belongs on this screen: the
	// question the screen answers is what the installation talks to, and
	// listing only what the console wrote would answer a different one.
	//
	// Only while a worker is still saying so. Workers restate what they hold
	// on every pass, so an observation that has stopped being refreshed is the
	// ghost of a process that is gone — and a ghost cannot be edited or
	// removed, which makes it a row nobody can get rid of.
	fresh := clockOr(s.clock).Now().Add(-staleObservation)
	for name, seen := range observed {
		if configured[name] || seen.ObservedAt.Before(fresh) {
			continue
		}
		body.McpServers = append(body.McpServers, openapi.MCPServer{
			Name: name, Enabled: true, Managed: ptr(false),
			Health: ptr(healthFrom(seen)),
		})
	}
	slices.SortFunc(body.McpServers, func(a, b openapi.MCPServer) int {
		return strings.Compare(a.Name, b.Name)
	})

	providers, err := s.integrations.Providers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	for _, p := range providers {
		body.Providers = append(body.Providers, openapi.ModelProvider{
			Name: p.Name, Kind: openapi.ModelProviderKind(p.Kind), BaseUrl: p.BaseURL,
			// Only whether a credential exists. The credential itself never
			// leaves the vault through this API.
			Enabled: p.Enabled, HasKey: p.HasKey,
			UpdatedBy: ptr(p.UpdatedBy), UpdatedAt: ptr(p.UpdatedAt),
		})
	}
	return body, nil
}

// knownProviders exposes the model package's preset table.
func knownProviders() []openapi.ModelPreset {
	names := model.PresetNames()
	out := make([]openapi.ModelPreset, 0, len(names))
	for _, name := range names {
		p, ok := model.Preset(name)
		if !ok {
			continue
		}
		out = append(out, openapi.ModelPreset{
			Name:           p.Name,
			Kind:           openapi.ModelPresetKind(p.Kind),
			BaseUrl:        ptr(p.BaseURL),
			SupportsEffort: ptr(p.SupportsReasoningEffort),
			Models:         &p.Models,
		})
	}
	return out
}
