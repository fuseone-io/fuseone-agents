package httpapi

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/settings"
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
	MCPServerNamed(ctx context.Context, name string) (domain.MCPServer, error)
	// MCPServerNamedIf asks allow with the server's stored scope before its
	// sealed document is opened.
	MCPServerNamedIf(ctx context.Context, name string, allow func(settings.ScopeKind, domain.Scope) error) (domain.MCPServer, error)
	// MCPServerStoredAt is where a server's row lives, the installation's for
	// a name no row holds (NT-019).
	MCPServerStoredAt(ctx context.Context, name string) (settings.ScopeKind, domain.Scope, error)
	PersonalCredentials
	Providers(ctx context.Context) ([]domain.ModelProvider, error)
	ProviderNamed(ctx context.Context, name string) (domain.ModelProvider, error)
	WriteMCPServer(ctx context.Context, w admin.MCPServerWrite) (domain.Revision, error)
	MCPCredentials(ctx context.Context, name string) (domain.MCPCredentials, error)
	RequestMCPProbe(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error
	// The ...At doors act on the exact row a door authorized (NT-019).
	RequestMCPProbeAt(ctx context.Context, by domain.UserID, at admin.MCPServerAt, name string) error
	RequestMCPRediscoveryAt(ctx context.Context, by domain.UserID, at admin.MCPServerAt, name string) (int64, error)
	MCPRediscoveryAt(ctx context.Context, at admin.MCPServerAt, name string) (domain.MCPRediscoveryStatus, error)
	RequestMCPRediscovery(ctx context.Context, by domain.UserID, scope domain.Scope, name string) (int64, error)
	MCPRediscovery(ctx context.Context, name string) (domain.MCPRediscoveryStatus, error)
	RemoveMCPServer(ctx context.Context, r admin.Removal) error
	WriteProvider(ctx context.Context, w admin.ProviderWrite) (domain.Revision, error)
	RemoveProvider(ctx context.Context, r admin.Removal) error
	// The platform's egress policies for servers outside the installation
	// (NT-019).
	PutMCPEgressPolicy(ctx context.Context, by domain.UserID, audit domain.Scope, p admin.MCPEgressPolicy) error
	DeleteMCPEgressPolicy(ctx context.Context, by domain.UserID, audit domain.Scope, at domain.Scope) error
	MCPEgressPolicies(ctx context.Context) ([]admin.MCPEgressPolicy, error)
}

func healthFrom(seen domain.IntegrationHealth) openapi.IntegrationHealth {
	out := openapi.IntegrationHealth{
		Reachable: seen.Reachable, ToolCount: seen.ToolCount,
		Detail: ptr(seen.Detail), ObservedAt: seen.ObservedAt,
		ObservedBy: ptr(seen.ObservedBy),
	}
	if seen.LastReachableAt != nil {
		out.LastReachableAt = seen.LastReachableAt
	}
	if seen.ToolCall != nil {
		out.ToolCall = &openapi.IntegrationToolCallHealth{
			Ok:         seen.ToolCall.OK,
			Code:       seen.ToolCall.Code,
			ObservedAt: seen.ToolCall.ObservedAt,
			ObservedBy: ptr(seen.ToolCall.ObservedBy),
			LastOkAt:   seen.ToolCall.LastOKAt,
		}
	}
	return out
}

func (s *Server) ListIntegrations(ctx context.Context, _ openapi.ListIntegrationsRequestObject) (openapi.ListIntegrationsResponseObject, error) {
	// A reader of the platform's, as before, or whoever configures in some
	// scope: an area's administrator lists its own servers and the ones it
	// inherits, and nothing of a sibling (NT-019).
	caller, allowed := mcpListingCaller(ctx)
	if !allowed {
		return openapi.ListIntegrations403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermToolRead, adminScope),
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
	callAuth, err := s.mcpCallAuth(ctx, servers)
	if err != nil {
		return nil, err
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
		listed, editable := mcpServerListing(caller, srv)
		if !listed {
			continue
		}
		server := mcpServerConfig(srv)
		server.Editable = ptr(editable)
		server.CallAuth = ptr(callAuth.forServer(srv.Name, srv.TransportOf()))
		server.Egress = ptr(s.mcpEgress(srv))
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
			Egress: ptr(openapi.MCPServerEgress{
				Policy: openapi.MCPServerEgressPolicyUnknown,
			}),
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
		body.Providers = append(body.Providers, modelProviderConfig(p))
	}
	return body, nil
}

type mcpCallAuth struct {
	known    map[string]bool
	personal map[string]bool
}

func (s *Server) mcpCallAuth(ctx context.Context, servers []domain.MCPServer) (mcpCallAuth, error) {
	out := mcpCallAuth{known: map[string]bool{}, personal: map[string]bool{}}
	if s.known != nil {
		for _, entry := range s.known.All() {
			out.known[entry.Server] = entry.RequiresPersonalCredential()
		}
	}
	caller, ok := auth.PrincipalFrom(ctx)
	if !ok || s.integrations == nil {
		return out, nil
	}
	personal, err := s.integrations.MCPPersonalCredentialPresence(
		ctx, caller.ID, remoteServerNames(servers))
	if err != nil {
		return mcpCallAuth{}, fmt.Errorf("list caller MCP credentials: %w", err)
	}
	out.personal = personal
	return out, nil
}

func remoteServerNames(servers []domain.MCPServer) []string {
	var out []string
	for _, server := range servers {
		if server.TransportOf() == domain.TransportHTTP {
			out = append(out, server.Name)
		}
	}
	return out
}

func (a mcpCallAuth) forServer(name, transport string) openapi.MCPServerCallAuth {
	policy := openapi.MCPServerCallAuthPolicyUnknown
	hasPersonal := a.personal[name]
	if transport == domain.TransportStdio {
		policy = openapi.MCPServerCallAuthPolicyLocalProcess
		hasPersonal = false
	} else if requiresPersonal, ok := a.known[name]; ok {
		if requiresPersonal {
			policy = openapi.MCPServerCallAuthPolicyPersonalRequired
		} else {
			policy = openapi.MCPServerCallAuthPolicyInstallationOrService
		}
	}
	return openapi.MCPServerCallAuth{
		Policy:                      policy,
		CallerHasPersonalCredential: hasPersonal,
	}
}

func (s *Server) mcpEgress(server domain.MCPServer) openapi.MCPServerEgress {
	policy := openapi.MCPServerEgressPolicyUnknown
	switch server.TransportOf() {
	case domain.TransportHTTP:
		policy = openapi.MCPServerEgressPolicyMetadataRefused
	case domain.TransportStdio:
		if server.StdioEgress != nil && server.StdioEgress.Mode == domain.MCPEgressProxied {
			if s.stdioEgressNetworkPolicyDeclared {
				policy = openapi.MCPServerEgressPolicyProxyWithNetworkPolicy
			} else {
				policy = openapi.MCPServerEgressPolicyProxyRequested
			}
		} else {
			policy = openapi.MCPServerEgressPolicyUnconstrainedLocalProcess
		}
	}
	return openapi.MCPServerEgress{Policy: policy}
}

func stdioEgressToResponse(egress *domain.MCPStdioEgress) *openapi.MCPStdioEgress {
	if egress == nil {
		return nil
	}
	out := openapi.MCPStdioEgress{Mode: openapi.MCPStdioEgressMode(egress.Mode)}
	var destinations []openapi.MCPEgressDestination
	for _, dest := range egress.AllowedDestinations {
		destinations = append(destinations, openapi.MCPEgressDestination{
			Host: dest.Host,
			Port: dest.Port,
		})
	}
	if destinations != nil {
		out.AllowedDestinations = &destinations
	}
	return &out
}

func rateLimitToResponse(limit *domain.MCPRateLimit) *openapi.MCPRateLimit {
	if limit == nil {
		return nil
	}
	return &openapi.MCPRateLimit{
		RatePerSecond: ptr(limit.RatePerSecond),
		Burst:         ptr(limit.Burst),
	}
}

func resultCacheToResponse(cache *domain.MCPResultCache) *openapi.MCPResultCache {
	if cache == nil {
		return nil
	}
	return &openapi.MCPResultCache{
		TtlSeconds: ptr(cache.TTLSeconds),
		MaxEntries: ptr(cache.MaxEntries),
	}
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
