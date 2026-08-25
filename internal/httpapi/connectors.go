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

func connectorInstanceInput(
	name string, body openapi.ConnectorInstanceInput,
) (settings.ScopeKind, domain.Scope, connectortools.Instance, *string, bool, error) {
	scope, err := connectorScope(body.ScopeKind, body.Company, body.Area)
	if err != nil {
		return "", domain.Scope{}, connectortools.Instance{}, nil, false, err
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	instance := connectortools.Instance{
		Name: name, Connector: strings.TrimSpace(body.Connector),
		Enabled: enabled, Vault: vaultConfigFromRequest(body.Vault),
	}
	clear := body.ClearToken != nil && *body.ClearToken
	return settings.ScopeKind(body.ScopeKind), scope, instance, body.Token, clear, nil
}

func connectorScope(kind openapi.ConnectorScopeKind, company, area *string) (domain.Scope, error) {
	if !kind.Valid() {
		return domain.Scope{}, fmt.Errorf("unknown connector scope kind %q", kind)
	}
	scope := scopeParams(company, area)
	switch kind {
	case openapi.ConnectorScopeKindInstallation:
		if scope.Company != "" || scope.Area != "" {
			return domain.Scope{}, fmt.Errorf("installation connector scope must not name company or area")
		}
	case openapi.ConnectorScopeKindCompany:
		if scope.Company == "" || scope.Area != "" {
			return domain.Scope{}, fmt.Errorf("company connector scope needs company only")
		}
	case openapi.ConnectorScopeKindArea:
		if scope.Company == "" || scope.Area == "" {
			return domain.Scope{}, fmt.Errorf("area connector scope needs company and area")
		}
	}
	return scope, nil
}

func vaultConfigFromRequest(in *openapi.ConnectorVaultConfig) connectortools.VaultConfig {
	if in == nil {
		return connectortools.VaultConfig{}
	}
	var namespace string
	if in.Namespace != nil {
		namespace = *in.Namespace
	}
	return connectortools.VaultConfig{
		Address: in.Address, Mount: in.Mount, Namespace: namespace,
		AllowedPathPrefixes: append([]string(nil), in.AllowedPathPrefixes...),
	}
}

func connectorInstanceResponse(instance connectortools.ConfiguredInstance) openapi.ConnectorInstance {
	item := openapi.ConnectorInstance{
		Name: instance.Name, Connector: instance.Connector, Enabled: instance.Enabled,
		HasToken: instance.HasToken, ScopeKind: openapi.ConnectorScopeKind(instance.ScopeKind),
		UpdatedBy: ptr(instance.UpdatedBy), UpdatedAt: ptr(instance.UpdatedAt),
	}
	company, area := responseScope(instance.ScopeKind, instance.Scope)
	item.Company, item.Area = company, area
	if instance.Connector == "vault" {
		item.Vault = ptr(vaultConfigToResponse(instance.Vault))
	}
	return item
}

func responseScope(kind settings.ScopeKind, scope domain.Scope) (*string, *string) {
	if kind == settings.ScopeInstallation {
		return nil, nil
	}
	company := string(scope.Company)
	if kind == settings.ScopeCompany {
		return &company, nil
	}
	area := string(scope.Area)
	return &company, &area
}

func vaultConfigToResponse(in connectortools.VaultConfig) openapi.ConnectorVaultConfig {
	out := openapi.ConnectorVaultConfig{
		Address: in.Address, Mount: in.Mount,
		AllowedPathPrefixes: append([]string(nil), in.AllowedPathPrefixes...),
	}
	if in.Namespace != "" {
		out.Namespace = ptr(in.Namespace)
	}
	return out
}
