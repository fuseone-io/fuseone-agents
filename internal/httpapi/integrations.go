package httpapi

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/settings"
)

// Configuring what the platform may talk to is governed by one permission.
//
// A tool server and a model provider are the same kind of decision: both widen
// what agents can reach outside the installation, and somebody trusted with
// one is trusted with the other.
const permConfigure = domain.PermProviderWrite

var errNoAdministration = errors.New("this installation has no administration store")

func (s *Server) PutMCPServer(ctx context.Context, req openapi.PutMCPServerRequestObject) (openapi.PutMCPServerResponseObject, error) {
	if forbidden := configuresSomewhere(ctx); forbidden != nil {
		return openapi.PutMCPServer403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}

	server := domain.MCPServer{Name: req.Name, Enabled: true}
	if req.Body.Transport != nil {
		server.Transport = string(*req.Body.Transport)
	}
	if req.Body.Command != nil {
		server.Command = *req.Body.Command
	}
	if req.Body.Args != nil {
		server.Args = *req.Body.Args
	}
	if req.Body.Url != nil {
		server.URL = *req.Body.Url
	}
	if req.Body.ProtocolMode != nil {
		server.ProtocolMode = string(*req.Body.ProtocolMode)
	}
	if req.Body.ConfigFileEnv != nil {
		server.ConfigFileEnv = req.Body.ConfigFileEnv
	}
	if req.Body.RateLimit != nil {
		server.RateLimit = rateLimitFromRequest(req.Body.RateLimit)
	}
	server.CallTimeout = req.Body.CallTimeoutSeconds
	if req.Body.Cache != nil {
		server.Cache = resultCacheFromRequest(req.Body.Cache)
	}
	if req.Body.StdioEgress != nil {
		server.StdioEgress = stdioEgressFromRequest(req.Body.StdioEgress)
	}
	if req.Body.Enabled != nil {
		server.Enabled = *req.Body.Enabled
	}
	// Absent leaves the choice as it stands; present replaces it, empty array
	// included. The same rule as the credential, for the same reason: not
	// mentioning something is not a request to remove it.
	if req.Body.Surface != nil {
		server.Surface = req.Body.Surface
	}
	if req.Body.AcceptsLocalExecution != nil {
		server.AcceptsLocalExecution = *req.Body.AcceptsLocalExecution
	}
	/*
		Only what this request named.

		An omitted token means "keep the stored one" and an omitted set of
		variables means the same, which is what lets somebody correct an
		address without re-entering a secret they do not have. Defaulting
		either to empty here would turn every edit into a quiet erasure.
	*/
	creds := domain.MCPCredentialPatch{
		Token:   req.Body.Token,
		Headers: stringMapOrNil(req.Body.Headers),
		OAuth:   oauthGrantFromRequest(req.Body.Oauth),
		// The key reaches the store and goes no further: it is sealed there,
		// and nothing on the way logs or echoes it.
		GitHubApp: githubAppFromRequest(req.Body.GithubApp),
	}
	if req.Body.Env != nil {
		creds.Env = *req.Body.Env
	}
	if req.Body.ConfigFile != nil {
		creds.ConfigFile = req.Body.ConfigFile
	}

	expect, err := domain.ParseIfMatch(req.Params.IfMatch)
	if err != nil {
		return openapi.PutMCPServer400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	kind, stored, err := connectorScopeOrInstallation(req.Body.ScopeKind, req.Body.Company, req.Body.Area)
	if err != nil {
		return openapi.PutMCPServer400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	caller, forbidden := s.configurerAt(ctx, kind, stored)
	if forbidden != nil {
		return openapi.PutMCPServer403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	revision, err := s.integrations.WriteMCPServer(ctx, admin.MCPServerWrite{
		By: caller, Scope: connectorRuntimeScope(kind, stored), At: kind, Stored: stored,
		Server: server, Given: creds, Expect: expect,
	})
	if stale, ok := staleFrom(err); ok {
		return openapi.PutMCPServer412ApplicationProblemPlusJSONResponse{
			PreconditionFailedApplicationProblemPlusJSONResponse: stale,
		}, nil
	}
	if errors.Is(err, settings.ErrNameTaken) {
		return openapi.PutMCPServer409ApplicationProblemPlusJSONResponse{
			NameTakenApplicationProblemPlusJSONResponse: openapi.NameTakenApplicationProblemPlusJSONResponse(nameTaken()),
		}, nil
	}
	if errors.Is(err, admin.ErrAreaServerIsRemote) {
		return openapi.PutMCPServer400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	if err != nil {
		return openapi.PutMCPServer400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				upstreamRefused(err.Error())),
		}, nil
	}
	return openapi.PutMCPServer204Response{Headers: openapi.PutMCPServer204ResponseHeaders{ETag: string(revision)}}, nil
}

func rateLimitFromRequest(limit *openapi.MCPRateLimit) *domain.MCPRateLimit {
	if limit == nil {
		return nil
	}
	var rate float64
	if limit.RatePerSecond != nil {
		rate = *limit.RatePerSecond
	}
	var burst int
	if limit.Burst != nil {
		burst = *limit.Burst
	}
	return &domain.MCPRateLimit{RatePerSecond: rate, Burst: burst}
}

func resultCacheFromRequest(cache *openapi.MCPResultCache) *domain.MCPResultCache {
	if cache == nil {
		return nil
	}
	var ttl int
	if cache.TtlSeconds != nil {
		ttl = *cache.TtlSeconds
	}
	var maxEntries int
	if cache.MaxEntries != nil {
		maxEntries = *cache.MaxEntries
	}
	return &domain.MCPResultCache{TTLSeconds: ttl, MaxEntries: maxEntries}
}

func stdioEgressFromRequest(egress *openapi.MCPStdioEgress) *domain.MCPStdioEgress {
	if egress == nil {
		return nil
	}
	out := domain.MCPStdioEgress{Mode: string(egress.Mode)}
	if egress.AllowedDestinations != nil {
		for _, dest := range *egress.AllowedDestinations {
			out.AllowedDestinations = append(out.AllowedDestinations, domain.MCPEgressDestination{
				Host: dest.Host,
				Port: dest.Port,
			})
		}
	}
	return &out
}

func (s *Server) ProbeMCPServer(ctx context.Context, req openapi.ProbeMCPServerRequestObject) (openapi.ProbeMCPServerResponseObject, error) {
	// Refused before the lookup for whoever configures nowhere; then at the
	// server's own scope, where a name no row holds is the installation's, so
	// an area hears the same about a sibling's server as about none (NT-019).
	if forbidden := configuresSomewhere(ctx); forbidden != nil {
		return openapi.ProbeMCPServer403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	caller, at, forbidden, err := s.mcpConfigurer(ctx, req.Name)
	if err != nil {
		return nil, err
	}
	if forbidden != nil {
		return openapi.ProbeMCPServer403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}

	if err := s.integrations.RequestMCPProbeAt(ctx, caller, at, req.Name); err != nil {
		return openapi.ProbeMCPServer400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				upstreamRefused(err.Error())),
		}, nil
	}
	return openapi.ProbeMCPServer202Response{}, nil
}

func oauthGrantFromRequest(in *openapi.MCPOAuthGrant) *domain.MCPOAuthGrant {
	if in == nil {
		return nil
	}
	grant := domain.MCPOAuthGrant{
		AccessToken:   valueOr(in.AccessToken),
		RefreshToken:  valueOr(in.RefreshToken),
		TokenURL:      valueOr(in.TokenURL),
		ClientID:      valueOr(in.ClientID),
		ClientSecret:  valueOr(in.ClientSecret),
		TokenType:     valueOr(in.TokenType),
		ExpiresAtUnix: valueOr(in.ExpiresAtUnix),
	}
	if in.Scopes != nil {
		grant.Scopes = append([]string(nil), (*in.Scopes)...)
	}
	return &grant
}

func (s *Server) DeleteMCPServer(ctx context.Context, req openapi.DeleteMCPServerRequestObject) (openapi.DeleteMCPServerResponseObject, error) {
	// Refused before the lookup for whoever configures nowhere; then at the
	// server's own scope, where a name no row holds is the installation's, so
	// an area hears the same about a sibling's server as about none (NT-019).
	if forbidden := configuresSomewhere(ctx); forbidden != nil {
		return openapi.DeleteMCPServer403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	caller, at, forbidden, err := s.mcpConfigurer(ctx, req.Name)
	if err != nil {
		return nil, err
	}
	if forbidden != nil {
		return openapi.DeleteMCPServer403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}

	expect, err := domain.ParseIfMatch(req.Params.IfMatch)
	if err != nil {
		return openapi.DeleteMCPServer400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	err = s.integrations.RemoveMCPServer(ctx, admin.Removal{
		By: caller, Scope: at.Audit, Name: req.Name, Expect: expect, At: at.Kind, Stored: at.Stored,
	})
	if stale, ok := staleFrom(err); ok {
		return openapi.DeleteMCPServer412ApplicationProblemPlusJSONResponse{
			PreconditionFailedApplicationProblemPlusJSONResponse: stale,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return openapi.DeleteMCPServer204Response{}, nil
}

// configurer checks the permission and returns who is acting, so every change
// is attributed to a person rather than to the platform.
func (s *Server) configurer(ctx context.Context) (domain.UserID, *openapi.ForbiddenApplicationProblemPlusJSONResponse) {
	if resp := s.refuse(ctx, permConfigure); resp != nil {
		return "", resp
	}
	caller, _ := auth.PrincipalFrom(ctx)
	return caller.ID, nil
}
