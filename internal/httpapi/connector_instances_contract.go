package httpapi

import (
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/settings"
)

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
		SQL: sqlConfigFromInput(body.Sql),
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
	if instance.Connector == "sql" {
		item.Sql = ptr(sqlConfigToResponse(instance.SQL))
	}
	return item
}

func connectorInstanceDetailResponse(instance connectortools.ConfiguredInstance) openapi.ConnectorInstanceDetail {
	item := openapi.ConnectorInstanceDetail{
		Name: instance.Name, Connector: instance.Connector, Enabled: instance.Enabled,
		HasToken: instance.HasToken, ScopeKind: openapi.ConnectorScopeKind(instance.ScopeKind),
		UpdatedBy: ptr(instance.UpdatedBy), UpdatedAt: ptr(instance.UpdatedAt),
	}
	item.Company, item.Area = responseScope(instance.ScopeKind, instance.Scope)
	if instance.Connector == "vault" {
		item.Vault = ptr(vaultConfigToResponse(instance.Vault))
	}
	if instance.Connector == "sql" {
		item.Sql = ptr(sqlConfigToInput(instance.SQL))
	}
	return item
}

func connectorInstanceAt(
	instance connectortools.ConfiguredInstance, name string,
	kind settings.ScopeKind, scope domain.Scope,
) bool {
	if instance.Name != name || instance.ScopeKind != kind {
		return false
	}
	switch kind {
	case settings.ScopeInstallation:
		return true
	case settings.ScopeCompany:
		return instance.Scope.Company == scope.Company
	case settings.ScopeArea:
		return instance.Scope == scope
	default:
		return false
	}
}

// sqlConfigToResponse is addressing plus the safe identity of the binding.
// Built field by field rather than by copying the struct: a field added to
// SQLConfig later must be a decision to expose it, not an inheritance.
func sqlConfigToResponse(cfg connectortools.SQLConfig) openapi.ConnectorSQLResponse {
	templates := make([]openapi.ConnectorSQLTemplateSummary, 0, len(cfg.Templates))
	for _, tpl := range cfg.Templates {
		templates = append(templates, openapi.ConnectorSQLTemplateSummary{
			Id: tpl.ID, Parameters: sqlParametersToResponse(tpl.Parameters),
			TimeoutSeconds: tpl.TimeoutSeconds, MaxRows: tpl.MaxRows, MaxBytes: tpl.MaxBytes,
		})
	}
	return openapi.ConnectorSQLResponse{
		Driver: openapi.ConnectorSQLResponseDriver(cfg.Driver),
		Host:   cfg.Host, Port: cfg.Port, Database: cfg.Database,
		Templates: templates,
		CredentialSource: openapi.ConnectorCredentialSource{
			Kind:          openapi.ConnectorCredentialSourceKind(cfg.CredentialSource.Kind),
			VaultInstance: cfg.CredentialSource.VaultInstance,
			Mount:         cfg.CredentialSource.Mount,
			Role:          cfg.CredentialSource.Role,
		},
	}
}

func sqlConfigFromInput(in *openapi.ConnectorSQLInput) connectortools.SQLConfig {
	if in == nil {
		return connectortools.SQLConfig{}
	}
	templates := make([]connectortools.SQLTemplate, 0, len(in.Templates))
	for _, tpl := range in.Templates {
		params := make([]connectortools.SQLParameter, 0, len(tpl.Parameters))
		for _, param := range tpl.Parameters {
			params = append(params, connectortools.SQLParameter{
				Name: param.Name, Type: connectortools.SQLParamType(param.Type),
			})
		}
		templates = append(templates, connectortools.SQLTemplate{
			ID: tpl.Id, SQL: tpl.Sql, Parameters: params,
			TimeoutSeconds: tpl.TimeoutSeconds, MaxRows: tpl.MaxRows, MaxBytes: tpl.MaxBytes,
		})
	}
	return connectortools.SQLConfig{
		Driver: connectortools.SQLDriver(in.Driver),
		Host:   in.Host, Port: in.Port, Database: in.Database,
		Templates: templates,
		CredentialSource: connectortools.CredentialSource{
			Kind:          connectortools.CredentialSourceKind(in.CredentialSource.Kind),
			VaultInstance: in.CredentialSource.VaultInstance,
			Mount:         in.CredentialSource.Mount,
			Role:          in.CredentialSource.Role,
		},
	}
}

func sqlConfigToInput(cfg connectortools.SQLConfig) openapi.ConnectorSQLInput {
	templates := make([]openapi.ConnectorSQLTemplate, 0, len(cfg.Templates))
	for _, tpl := range cfg.Templates {
		templates = append(templates, openapi.ConnectorSQLTemplate{
			Id: tpl.ID, Sql: tpl.SQL, Parameters: sqlParametersToResponse(tpl.Parameters),
			TimeoutSeconds: tpl.TimeoutSeconds, MaxRows: tpl.MaxRows, MaxBytes: tpl.MaxBytes,
		})
	}
	return openapi.ConnectorSQLInput{
		Driver: openapi.ConnectorSQLInputDriver(cfg.Driver),
		Host:   cfg.Host, Port: cfg.Port, Database: cfg.Database,
		CredentialSource: openapi.ConnectorCredentialSource{
			Kind:          openapi.ConnectorCredentialSourceKind(cfg.CredentialSource.Kind),
			VaultInstance: cfg.CredentialSource.VaultInstance,
			Mount:         cfg.CredentialSource.Mount, Role: cfg.CredentialSource.Role,
		},
		Templates: templates,
	}
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

func sqlParametersToResponse(in []connectortools.SQLParameter) []openapi.ConnectorSQLParameter {
	out := make([]openapi.ConnectorSQLParameter, 0, len(in))
	for _, param := range in {
		out = append(out, openapi.ConnectorSQLParameter{
			Name: param.Name, Type: openapi.ConnectorSQLParameterType(param.Type),
		})
	}
	return out
}
