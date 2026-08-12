package httpapi

import (
	"context"
	"errors"
	"net/http"

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

	server := domain.MCPServer{Name: req.Name, Command: req.Body.Command, Enabled: true}
	if req.Body.Args != nil {
		server.Args = *req.Body.Args
	}
	if req.Body.Enabled != nil {
		server.Enabled = *req.Body.Enabled
	}

	if err := s.integrations.PutMCPServer(ctx, caller, adminScope, server); err != nil {
		return openapi.PutMCPServer400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Não foi possível configurar", err.Error())),
		}, nil
	}
	return openapi.PutMCPServer204Response{}, nil
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
				problem(http.StatusBadRequest, "Não foi possível configurar", err.Error())),
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
