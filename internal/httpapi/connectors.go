package httpapi

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/connectors"
	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/settings"
)

type ConnectorInstances interface {
	ConnectorInstances(ctx context.Context) ([]connectortools.ConfiguredInstance, error)
	PutConnectorInstance(
		ctx context.Context, by domain.UserID, scope domain.Scope,
		scopeKind settings.ScopeKind, instanceScope domain.Scope,
		instance connectortools.Instance, token *string, clearToken bool,
	) error
	DeleteConnectorInstance(
		ctx context.Context, by domain.UserID, scope domain.Scope,
		scopeKind settings.ScopeKind, instanceScope domain.Scope, name string,
	) error
}

// ListConnectorCatalog exposes governed connector shapes. It does not configure
// an instance by itself; executable tools exist only for configured instances.
func (s *Server) ListConnectorCatalog(ctx context.Context, _ openapi.ListConnectorCatalogRequestObject) (openapi.ListConnectorCatalogResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermToolClassify); resp != nil {
		return openapi.ListConnectorCatalog403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}

	catalog := connectors.Catalog()
	items := make([]openapi.GovernedConnector, 0, len(catalog))
	for _, c := range catalog {
		item := openapi.GovernedConnector{
			Id:         c.ID,
			Name:       c.Name,
			Category:   openapi.GovernedConnectorCategory(c.Category),
			Summary:    c.Summary,
			Maturity:   openapi.GovernedConnectorMaturity(c.Maturity),
			Guarantees: slices.Clone(c.Guarantees),
			Caveats:    slices.Clone(c.Caveats),
			Operations: connectorOperations(c.Operations),
		}
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b openapi.GovernedConnector) int {
		return strings.Compare(a.Name, b.Name)
	})
	return openapi.ListConnectorCatalog200JSONResponse{Items: items}, nil
}

func (s *Server) ListConnectorInstances(ctx context.Context, _ openapi.ListConnectorInstancesRequestObject) (openapi.ListConnectorInstancesResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermToolRead); resp != nil {
		return openapi.ListConnectorInstances403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.connectors == nil {
		return openapi.ListConnectorInstances200JSONResponse{Items: []openapi.ConnectorInstance{}}, nil
	}
	configured, err := s.connectors.ConnectorInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("list connector instances: %w", err)
	}
	items := make([]openapi.ConnectorInstance, 0, len(configured))
	for _, instance := range configured {
		items = append(items, connectorInstanceResponse(instance))
	}
	slices.SortFunc(items, func(a, b openapi.ConnectorInstance) int {
		if a.Connector != b.Connector {
			return strings.Compare(a.Connector, b.Connector)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return openapi.ListConnectorInstances200JSONResponse{Items: items}, nil
}

// GetConnectorInstance returns the authored configuration to a configurer.
// The ordinary list deliberately omits SQL text because tool readers do not
// need to know table names and filters; editing does, or changing a host would
// silently replace every registered query with an incomplete summary.
func (s *Server) GetConnectorInstance(ctx context.Context, req openapi.GetConnectorInstanceRequestObject) (openapi.GetConnectorInstanceResponseObject, error) {
	if _, forbidden := s.configurer(ctx); forbidden != nil {
		return openapi.GetConnectorInstance403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.connectors == nil {
		return nil, errNoAdministration
	}
	scope, err := connectorScope(req.Params.ScopeKind, req.Params.Company, req.Params.Area)
	if err != nil {
		return openapi.GetConnectorInstance400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	configured, err := s.connectors.ConnectorInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("get connector instance: %w", err)
	}
	for _, instance := range configured {
		if connectorInstanceAt(instance, req.Name, settings.ScopeKind(req.Params.ScopeKind), scope) {
			return openapi.GetConnectorInstance200JSONResponse(
				connectorInstanceDetailResponse(instance)), nil
		}
	}
	return openapi.GetConnectorInstance404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.Name),
	}, nil
}

func (s *Server) PutConnectorInstance(ctx context.Context, req openapi.PutConnectorInstanceRequestObject) (openapi.PutConnectorInstanceResponseObject, error) {
	caller, forbidden := s.configurer(ctx)
	if forbidden != nil {
		return openapi.PutConnectorInstance403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.connectors == nil {
		return nil, errNoAdministration
	}
	if req.Body == nil {
		return openapi.PutConnectorInstance400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("connector instance body is required")),
		}, nil
	}
	scopeKind, scope, instance, token, clearToken, err := connectorInstanceInput(req.Name, *req.Body)
	if err != nil {
		return openapi.PutConnectorInstance400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	if err := s.connectors.PutConnectorInstance(ctx, caller, adminScope, scopeKind, scope, instance, token, clearToken); err != nil {
		return openapi.PutConnectorInstance400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				notStored(err.Error())),
		}, nil
	}
	return openapi.PutConnectorInstance204Response{}, nil
}

func (s *Server) DeleteConnectorInstance(ctx context.Context, req openapi.DeleteConnectorInstanceRequestObject) (openapi.DeleteConnectorInstanceResponseObject, error) {
	caller, forbidden := s.configurer(ctx)
	if forbidden != nil {
		return openapi.DeleteConnectorInstance403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.connectors == nil {
		return nil, errNoAdministration
	}
	scope, err := connectorScope(req.Params.ScopeKind, req.Params.Company, req.Params.Area)
	if err != nil {
		return openapi.DeleteConnectorInstance400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	scopeKind := settings.ScopeKind(req.Params.ScopeKind)
	if err := s.connectors.DeleteConnectorInstance(ctx, caller, adminScope, scopeKind, scope, req.Name); err != nil {
		return openapi.DeleteConnectorInstance400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				notStored(err.Error())),
		}, nil
	}
	return openapi.DeleteConnectorInstance204Response{}, nil
}

func connectorOperations(in []connectors.Operation) []openapi.GovernedConnectorOperation {
	out := make([]openapi.GovernedConnectorOperation, 0, len(in))
	for _, op := range in {
		out = append(out, openapi.GovernedConnectorOperation{
			Id:             op.ID,
			Name:           op.Name,
			Summary:        op.Summary,
			Effects:        connectorEffects(op.Effects),
			Approval:       openapi.ConnectorApproval(op.Approval),
			SecretHandling: openapi.ConnectorSecretHandling(op.SecretHandling),
			// The effective policy, never the raw field: the contract reports
			// what the platform will do, and an operation that never decided
			// must read as `never` rather than as an empty string.
			CachePolicy: openapi.ConnectorCachePolicy(op.EffectiveCachePolicy()),
		})
	}
	return out
}

func connectorEffects(in []connectors.Effect) []openapi.ConnectorEffect {
	out := make([]openapi.ConnectorEffect, 0, len(in))
	for _, e := range in {
		out = append(out, openapi.ConnectorEffect(e))
	}
	return out
}
