package httpapi

import (
	"context"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

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
	if req.Body.Models != nil {
		provider.Models = *req.Body.Models
	}
	if req.Body.AutoToolChoiceModels != nil {
		provider.AutoToolChoiceModels = *req.Body.AutoToolChoiceModels
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

	expect, err := domain.ParseIfMatch(req.Params.IfMatch)
	if err != nil {
		return openapi.PutModelProvider400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	revision, err := s.integrations.WriteProvider(ctx, admin.ProviderWrite{
		By: caller, Scope: adminScope, Provider: provider, APIKey: apiKey, Expect: expect,
	})
	if stale, ok := staleFrom(err); ok {
		return openapi.PutModelProvider412ApplicationProblemPlusJSONResponse{
			PreconditionFailedApplicationProblemPlusJSONResponse: stale,
		}, nil
	}
	if err != nil {
		return openapi.PutModelProvider400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				notStored(err.Error())),
		}, nil
	}
	return openapi.PutModelProvider204Response{Headers: openapi.PutModelProvider204ResponseHeaders{ETag: string(revision)}}, nil
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

	expect, err := domain.ParseIfMatch(req.Params.IfMatch)
	if err != nil {
		return openapi.DeleteModelProvider400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	err = s.integrations.RemoveProvider(ctx, admin.Removal{By: caller, Scope: adminScope, Name: req.Name, Expect: expect})
	if stale, ok := staleFrom(err); ok {
		return openapi.DeleteModelProvider412ApplicationProblemPlusJSONResponse{
			PreconditionFailedApplicationProblemPlusJSONResponse: stale,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return openapi.DeleteModelProvider204Response{}, nil
}
