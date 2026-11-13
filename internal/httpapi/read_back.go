package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/policy"
	"github.com/fuseone/agents/internal/settings"
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
	if forbidden := configuresSomewhere(ctx); forbidden != nil {
		return openapi.GetMCPServer403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	// One read, then the decision on what it found: a lookup before the read
	// would let a write land between the two, and the answer would not be the
	// state the read began on. A name no row holds is the installation's, so
	// an area hears the same about a sibling's server as about none (NT-019).
	// The decision is made on the row's scope before its sealed document is
	// opened, so a refused caller never reaches the vault: a credential that
	// does not open would otherwise answer differently from a name nobody holds.
	var refused *openapi.ForbiddenApplicationProblemPlusJSONResponse
	server, err := s.integrations.MCPServerNamedIf(ctx, req.Name, func(kind settings.ScopeKind, stored domain.Scope) error {
		if _, refused = s.configurerAt(ctx, kind, stored); refused != nil {
			return errMCPServerRefused
		}
		return nil
	})
	if refused != nil {
		return openapi.GetMCPServer403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	unknown := errors.Is(err, admin.ErrMCPServerUnknown)
	if err != nil && !unknown {
		return nil, fmt.Errorf("get MCP server: %w", err)
	}
	if unknown {
		// A name nobody holds is the installation's: an area hears 403, as it
		// does for a sibling's server.
		if _, forbidden := s.configurerAt(ctx, settings.ScopeInstallation, domain.Scope{}); forbidden != nil {
			return openapi.GetMCPServer403ApplicationProblemPlusJSONResponse{
				ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
			}, nil
		}
		return openapi.GetMCPServer404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.Name),
		}, nil
	}
	out := mcpServerConfig(server)
	out.SecretFingerprint = someString(server.SecretFingerprint)
	applyCredentialSummary(&out, server.Credential)
	return openapi.GetMCPServer200JSONResponse{
		Body: out, Headers: openapi.GetMCPServer200ResponseHeaders{ETag: string(server.Revision)},
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
	provider, err := s.integrations.ProviderNamed(ctx, req.Name)
	if errors.Is(err, admin.ErrProviderUnknown) {
		return openapi.GetModelProvider404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.Name),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get model provider: %w", err)
	}
	out := modelProviderConfig(provider)
	out.SecretFingerprint = someString(provider.SecretFingerprint)
	return openapi.GetModelProvider200JSONResponse{
		Body: out, Headers: openapi.GetModelProvider200ResponseHeaders{ETag: string(provider.Revision)},
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
	found, bound, err := s.channels.Named(ctx, req.Name)
	if errors.Is(err, admin.ErrChannelUnknown) {
		return openapi.GetChannel404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.Name),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	out := channelFrom(found, bound, nil)
	out.SecretFingerprint = someString(found.SecretFingerprint)
	// Left out, not empty. Who interacted changes with nobody having
	// written anything, and an empty list would say nobody did.
	out.SeenAccounts = nil
	return openapi.GetChannel200JSONResponse{
		Body: out, Headers: openapi.GetChannel200ResponseHeaders{ETag: string(found.Revision)},
	}, nil
}

// GetPolicy takes what the list takes, not what the write takes. A policy
// constrains people who hold nothing in the administrative scope, and being
// stopped by a rule you cannot read is unactionable.
func (s *Server) GetPolicy(ctx context.Context, req openapi.GetPolicyRequestObject) (openapi.GetPolicyResponseObject, error) {
	if len(auth.VisibleScopes(ctx, domain.PermPolicyRead)) == 0 {
		return openapi.GetPolicy403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermPolicyRead, domain.Scope{}),
		}, nil
	}
	if s.policies == nil {
		return nil, errNoAdministration
	}
	found, err := s.policies.Named(ctx, req.Code)
	if errors.Is(err, policy.ErrNoPolicy) {
		return openapi.GetPolicy404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.Code),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get policy: %w", err)
	}
	return openapi.GetPolicy200JSONResponse{
		Body:    policyFrom(found),
		Headers: openapi.GetPolicy200ResponseHeaders{ETag: string(policy.RevisionOf(found))},
	}, nil
}

func applyCredentialSummary(out *openapi.MCPServer, summary *domain.MCPCredentialSummary) {
	if summary == nil {
		return
	}
	if len(summary.HeaderNames) > 0 {
		out.HeaderNames = &summary.HeaderNames
	}
	if len(summary.EnvNames) > 0 {
		out.EnvNames = &summary.EnvNames
	}
	if client := summary.OAuth; client != nil {
		out.OauthClient = &openapi.MCPOAuthClient{
			TokenURL: someString(client.TokenURL), ClientID: someString(client.ClientID),
			TokenType: someString(client.TokenType),
		}
		if len(client.Scopes) > 0 {
			out.OauthClient.Scopes = &client.Scopes
		}
	}
}

// mcpServerConfig is a server as it was configured, before anything observed
// about it is added.
func mcpServerConfig(srv domain.MCPServer) openapi.MCPServer {
	kind, stored := mcpServerStored(srv.Scope)
	company, area := responseScope(kind, stored)
	server := openapi.MCPServer{
		Name: srv.Name, Enabled: srv.Enabled,
		// Where it is stored, so a client that sends back what it read writes
		// the same row (NT-019).
		ScopeKind: ptr(openapi.ConnectorScopeKind(kind)), Company: company, Area: area,
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
