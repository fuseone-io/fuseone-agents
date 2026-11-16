package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/settings"
)

// ProviderConfiguration is what the provider doors need of the store,
// declared here by the consumer.
type ProviderConfiguration interface {
	Providers(ctx context.Context) ([]domain.ModelProvider, error)
	ProviderNamed(ctx context.Context, name string) (domain.ModelProvider, error)
	// ProviderNamedIf asks allow with the row's stored scope before its key
	// is opened (NT-019).
	ProviderNamedIf(ctx context.Context, name string, allow func(settings.ScopeKind, domain.Scope) error) (domain.ModelProvider, error)
	// ProviderStoredAt is where a provider's row lives, the installation's
	// for a name no row holds.
	ProviderStoredAt(ctx context.Context, name string) (settings.ScopeKind, domain.Scope, error)
	WriteProvider(ctx context.Context, w admin.ProviderWrite) (domain.Revision, error)
	RemoveProvider(ctx context.Context, r admin.Removal) error
}

// PutModelProvider stores a provider at the scope the request names (NT-019,
// phase 3 writers), for whoever holds provider:write there.
func (s *Server) PutModelProvider(ctx context.Context, req openapi.PutModelProviderRequestObject) (openapi.PutModelProviderResponseObject, error) {
	if forbidden := configuresSomewhere(ctx); forbidden != nil {
		return openapi.PutModelProvider403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: *forbidden}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	kind, stored, err := connectorScopeOrInstallation(req.Body.ScopeKind, req.Body.Company, req.Body.Area)
	if err != nil {
		return openapi.PutModelProvider400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	caller, forbidden := s.configurerAt(ctx, kind, stored)
	if forbidden != nil {
		return openapi.PutModelProvider403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: *forbidden}, nil
	}
	// An environment provider is the installation's under that name, and
	// every scope reaches it: a company or an area may not take the name,
	// which would remove it from every other scope. A write at the
	// installation may still replace it, deliberately.
	if kind != settings.ScopeInstallation && s.providerNames != nil && s.providerNames.FromEnvironment(req.Name) {
		return openapi.PutModelProvider409ApplicationProblemPlusJSONResponse{
			NameTakenApplicationProblemPlusJSONResponse: openapi.NameTakenApplicationProblemPlusJSONResponse(nameTaken()),
		}, nil
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
		By: caller, Scope: connectorRuntimeScope(kind, stored), Provider: provider, APIKey: apiKey, Expect: expect,
		At: kind, Stored: stored,
	})
	if stale, ok := staleFrom(err); ok {
		return openapi.PutModelProvider412ApplicationProblemPlusJSONResponse{
			PreconditionFailedApplicationProblemPlusJSONResponse: stale,
		}, nil
	}
	if errors.Is(err, settings.ErrNameTaken) {
		return openapi.PutModelProvider409ApplicationProblemPlusJSONResponse{
			NameTakenApplicationProblemPlusJSONResponse: openapi.NameTakenApplicationProblemPlusJSONResponse(nameTaken()),
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

// DeleteModelProvider removes a provider at its own scope, for whoever holds
// provider:write there (NT-019).
func (s *Server) DeleteModelProvider(ctx context.Context, req openapi.DeleteModelProviderRequestObject) (openapi.DeleteModelProviderResponseObject, error) {
	if forbidden := configuresSomewhere(ctx); forbidden != nil {
		return openapi.DeleteModelProvider403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: *forbidden}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	kind, stored, err := s.integrations.ProviderStoredAt(ctx, req.Name)
	if err != nil {
		return nil, fmt.Errorf("find provider %s: %w", req.Name, err)
	}
	caller, forbidden := s.configurerAt(ctx, kind, stored)
	if forbidden != nil {
		return openapi.DeleteModelProvider403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: *forbidden}, nil
	}

	expect, err := domain.ParseIfMatch(req.Params.IfMatch)
	if err != nil {
		return openapi.DeleteModelProvider400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	err = s.integrations.RemoveProvider(ctx, admin.Removal{
		By: caller, Scope: connectorRuntimeScope(kind, stored), Name: req.Name, Expect: expect, At: kind, Stored: stored,
	})
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
