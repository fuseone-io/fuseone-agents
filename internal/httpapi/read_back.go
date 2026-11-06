package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Reading one resource by name, for a client that manages it declaratively.

Each read answers what a PUT stored, so a client can tell whether a resource
drifted without reading a whole listing. The authority is checked before the
lookup, so a refusal says nothing about whether the name exists, and a secret
reports only that one is stored.
*/

// GetMCPServer answers a server configured here. One a worker reports from a
// flag is on the listing, and has no stored form to compare with.
func (s *Server) GetMCPServer(ctx context.Context, req openapi.GetMCPServerRequestObject) (openapi.GetMCPServerResponseObject, error) {
	if _, forbidden := s.configurer(ctx); forbidden != nil {
		return openapi.GetMCPServer403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	servers, err := s.integrations.MCPServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("get MCP server: %w", err)
	}
	for _, server := range servers {
		if server.Name == req.Name {
			return openapi.GetMCPServer200JSONResponse(mcpServerConfig(server)), nil
		}
	}
	return openapi.GetMCPServer404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.Name),
	}, nil
}

func (s *Server) GetModelProvider(ctx context.Context, req openapi.GetModelProviderRequestObject) (openapi.GetModelProviderResponseObject, error) {
	if _, forbidden := s.configurer(ctx); forbidden != nil {
		return openapi.GetModelProvider403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	providers, err := s.integrations.Providers(ctx)
	if err != nil {
		return nil, fmt.Errorf("get model provider: %w", err)
	}
	for _, provider := range providers {
		if provider.Name == req.Name {
			return openapi.GetModelProvider200JSONResponse(modelProviderConfig(provider)), nil
		}
	}
	return openapi.GetModelProvider404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.Name),
	}, nil
}

func (s *Server) GetChannel(ctx context.Context, req openapi.GetChannelRequestObject) (openapi.GetChannelResponseObject, error) {
	if _, forbidden := s.configurer(ctx); forbidden != nil {
		return openapi.GetChannel403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.channels == nil {
		return nil, errNoAdministration
	}
	configured, err := s.channels.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	for _, c := range configured {
		if c.Name != req.Name {
			continue
		}
		bound, err := s.channels.Identities(ctx)
		if err != nil {
			return nil, fmt.Errorf("get channel identities: %w", err)
		}
		out := channelFrom(c, bound, nil)
		// Left out, not empty. Who interacted changes with nobody having
		// written anything, and an empty list would say nobody did.
		out.SeenAccounts = nil
		return openapi.GetChannel200JSONResponse(out), nil
	}
	return openapi.GetChannel404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.Name),
	}, nil
}

// mcpServerConfig is a server as it was configured, before anything observed
// about it is added.
func mcpServerConfig(srv domain.MCPServer) openapi.MCPServer {
	server := openapi.MCPServer{
		Name: srv.Name, Enabled: srv.Enabled,
		Transport:          ptr(openapi.Transport(srv.TransportOf())),
		ProtocolMode:       ptr(openapi.MCPProtocolMode(srv.MCPProtocolModeOf())),
		RateLimit:          rateLimitToResponse(srv.RateLimit),
		CallTimeoutSeconds: srv.CallTimeout,
		Cache:              resultCacheToResponse(srv.Cache),
		HasSecret:          ptr(srv.HasSecret),
		HasOAuth:           ptr(srv.HasOAuth),
		GithubApp:          githubAppToResponse(srv.GitHubApp),
		HasVariables:       ptr(srv.HasVariables),
		HasConfigFile:      ptr(srv.HasConfigFile),
		Managed:            ptr(true),
		UpdatedBy:          ptr(srv.UpdatedBy), UpdatedAt: ptr(srv.UpdatedAt),
		// What was brought in, and who accepted running it. Both are
		// stored and neither reached the screen, so a server somebody had
		// narrowed read back as all-in — and saving anything from that
		// screen would have widened it back.
		//
		// The pointer is the whole point: nil is "nobody has chosen", and
		// an empty list is "chosen, and none of it".
		Surface:               srv.Surface,
		AcceptsLocalExecution: ptr(srv.AcceptsLocalExecution),
		StdioEgress:           stdioEgressToResponse(srv.StdioEgress),
	}
	// Absent rather than null: the schema says array, and a pointer to a nil
	// slice encodes as neither.
	if len(srv.Args) > 0 {
		server.Args = &srv.Args
	}
	server.Command = someString(srv.Command)
	server.Url = someString(srv.URL)
	if srv.ConfigFileEnv != nil {
		server.ConfigFileEnv = someString(*srv.ConfigFileEnv)
	}
	return server
}

func modelProviderConfig(p domain.ModelProvider) openapi.ModelProvider {
	return openapi.ModelProvider{
		Name: p.Name, Kind: openapi.ModelProviderKind(p.Kind), BaseUrl: p.BaseURL,
		// An empty array and never null: a provider with no list is a
		// provider that offers none, and the schema says array or absent.
		Models:               ptr(append([]string{}, p.Models...)),
		AutoToolChoiceModels: ptr(append([]string{}, p.AutoToolChoiceModels...)),
		// Only whether a credential exists. The credential itself never
		// leaves the vault through this API.
		Enabled: p.Enabled, HasKey: p.HasKey,
		UpdatedBy: ptr(p.UpdatedBy), UpdatedAt: ptr(p.UpdatedAt),
	}
}
