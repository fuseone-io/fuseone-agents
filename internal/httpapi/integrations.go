package httpapi

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Configuring what the platform may talk to is governed by one permission.
//
// A tool server and a model provider are the same kind of decision: both widen
// what agents can reach outside the installation, and somebody trusted with
// one is trusted with the other.
const permConfigure = domain.PermProviderWrite

var errNoAdministration = errors.New("this installation has no administration store")

func (s *Server) PutMCPServer(ctx context.Context, req openapi.PutMCPServerRequestObject) (openapi.PutMCPServerResponseObject, error) {
	caller, forbidden := s.configurer(ctx)
	if forbidden != nil {
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
	}
	if req.Body.Env != nil {
		creds.Env = *req.Body.Env
	}
	if req.Body.ConfigFile != nil {
		creds.ConfigFile = req.Body.ConfigFile
	}

	if err := s.integrations.PutMCPServer(ctx, caller, adminScope, server, creds); err != nil {
		return openapi.PutMCPServer400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				upstreamRefused(err.Error())),
		}, nil
	}
	return openapi.PutMCPServer204Response{}, nil
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

func (s *Server) ProbeMCPServer(ctx context.Context, req openapi.ProbeMCPServerRequestObject) (openapi.ProbeMCPServerResponseObject, error) {
	caller, forbidden := s.configurer(ctx)
	if forbidden != nil {
		return openapi.ProbeMCPServer403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}

	if err := s.integrations.RequestMCPProbe(ctx, caller, adminScope, req.Name); err != nil {
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
	caller, forbidden := s.configurer(ctx)
	if forbidden != nil {
		return openapi.DeleteMCPServer403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}

	if err := s.integrations.DeleteMCPServer(ctx, caller, adminScope, req.Name); err != nil {
		return nil, err
	}
	return openapi.DeleteMCPServer204Response{}, nil
}

func (s *Server) PutModelProvider(ctx context.Context, req openapi.PutModelProviderRequestObject) (openapi.PutModelProviderResponseObject, error) {
	caller, forbidden := s.configurer(ctx)
	if forbidden != nil {
		return openapi.PutModelProvider403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}

	provider := domain.ModelProvider{
		Name: req.Name, Kind: string(req.Body.Kind), BaseURL: valueOr(req.Body.BaseUrl), Enabled: true,
	}
	if req.Body.Enabled != nil {
		provider.Enabled = *req.Body.Enabled
	}

	// An absent key keeps the stored one. The credential is sealed on arrival
	// and never comes back out through this API.
	var apiKey string
	if req.Body.ApiKey != nil {
		apiKey = *req.Body.ApiKey
	}

	if err := s.integrations.PutProvider(ctx, caller, adminScope, provider, apiKey); err != nil {
		return openapi.PutModelProvider400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				notStored(err.Error())),
		}, nil
	}
	return openapi.PutModelProvider204Response{}, nil
}

func (s *Server) DeleteModelProvider(ctx context.Context, req openapi.DeleteModelProviderRequestObject) (openapi.DeleteModelProviderResponseObject, error) {
	caller, forbidden := s.configurer(ctx)
	if forbidden != nil {
		return openapi.DeleteModelProvider403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}

	if err := s.integrations.DeleteProvider(ctx, caller, adminScope, req.Name); err != nil {
		return nil, err
	}
	return openapi.DeleteModelProvider204Response{}, nil
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
