package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

var ErrConnectorNeedsToken = errors.New("admin: an enabled connector instance needs a token")

type ConnectorInstances struct {
	pool     *pgxpool.Pool
	settings *settings.Store
}

func NewConnectorInstances(pool *pgxpool.Pool, store *settings.Store) *ConnectorInstances {
	return &ConnectorInstances{pool: pool, settings: store}
}

func (c *ConnectorInstances) ConnectorInstances(ctx context.Context) ([]connectortools.ConfiguredInstance, error) {
	return connectortools.NewSettings(c.settings).Configured(ctx)
}

func (c *ConnectorInstances) PutConnectorInstance(
	ctx context.Context, by domain.UserID, _ domain.Scope,
	scopeKind settings.ScopeKind, scope domain.Scope, instance connectortools.Instance,
	token *string, clearToken bool,
) error {
	storedScope, runtimeScope, err := connectorScopes(scopeKind, scope)
	if err != nil {
		return err
	}
	instance.Scope = runtimeScope
	needsToken := connectortools.RequiresToken(instance.Connector)
	if needsToken && instance.Enabled && token != nil && strings.TrimSpace(*token) == "" {
		return ErrConnectorNeedsToken
	}
	if err := connectortools.ValidateInstanceConfig(instance); err != nil {
		return err
	}
	// A binding names another instance, so it can only be judged against the
	// set. Reads settings, not Vault: saving a configuration must not reach the
	// system it configures, or an operator cannot write down an intention
	// before the thing it points at is reachable.
	if err := c.validateAgainstConfigured(ctx, instance, scopeKind, storedScope); err != nil {
		return err
	}
	value, err := connectortools.SettingValue(instance)
	if err != nil {
		return err
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	hasSecret, err := connectorHasSecretTx(ctx, tx, scopeKind, storedScope, instance.Name)
	if err != nil {
		return err
	}
	secret := ""
	if token != nil {
		secret = *token
	}
	clear := clearToken && secret == ""
	willHaveSecret := secret != "" || (!clear && hasSecret)
	if needsToken && instance.Enabled && !willHaveSecret {
		return ErrConnectorNeedsToken
	}
	if err := c.settings.PutTx(ctx, tx, settings.Setting{
		ScopeKind: scopeKind, Scope: storedScope,
		Kind: settings.KindConnectorInstance, Name: instance.Name,
		Value: value, Secret: secret, ClearSecret: clear,
		Enabled: instance.Enabled, UpdatedBy: string(by),
	}); err != nil {
		return err
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: runtimeScope, Action: "connector_instance.configured",
		Target: instance.Name, Detail: connectorInstanceDetail(instance, token != nil, clear),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (c *ConnectorInstances) DeleteConnectorInstance(
	ctx context.Context, by domain.UserID, _ domain.Scope,
	scopeKind settings.ScopeKind, scope domain.Scope, name string,
) error {
	if !connectortools.ValidInstanceName(name) {
		return fmt.Errorf("admin: invalid connector instance name %q", name)
	}
	storedScope, runtimeScope, err := connectorScopes(scopeKind, scope)
	if err != nil {
		return err
	}
	return removeScopedSetting(ctx, c.pool, c.settings, by,
		scopeKind, storedScope, runtimeScope,
		settings.KindConnectorInstance, name, "connector_instance.removed")
}

func connectorHasSecretTx(
	ctx context.Context, tx pgx.Tx, scopeKind settings.ScopeKind, scope domain.Scope, name string,
) (bool, error) {
	var hasSecret bool
	err := tx.QueryRow(ctx, `
		select secret is not null
		from settings
		where scope_kind = $1 and company_id = $2 and area_id = $3 and kind = $4 and name = $5
		for update`,
		string(scopeKind), string(scope.Company), string(scope.Area),
		string(settings.KindConnectorInstance), name,
	).Scan(&hasSecret)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("admin: read connector instance %s: %w", name, err)
	}
	return hasSecret, nil
}

func connectorScopes(scopeKind settings.ScopeKind, scope domain.Scope) (domain.Scope, domain.Scope, error) {
	switch scopeKind {
	case settings.ScopeInstallation:
		if scope.Company != "" || scope.Area != "" {
			return domain.Scope{}, domain.Scope{}, fmt.Errorf("admin: installation connector scope must not name company or area")
		}
		return domain.Scope{}, domain.Scope{Company: domain.Installation}, nil
	case settings.ScopeCompany:
		if scope.Company == "" || scope.Company == domain.Installation || scope.Area != "" {
			return domain.Scope{}, domain.Scope{}, fmt.Errorf("admin: connector company scope needs one company")
		}
		return domain.Scope{Company: scope.Company}, domain.Scope{Company: scope.Company}, nil
	case settings.ScopeArea:
		if scope.Company == "" || scope.Company == domain.Installation || scope.Area == "" {
			return domain.Scope{}, domain.Scope{}, fmt.Errorf("admin: connector area scope needs company and area")
		}
		return scope, scope, nil
	default:
		return domain.Scope{}, domain.Scope{}, fmt.Errorf("admin: unknown connector scope kind %q", scopeKind)
	}
}

func connectorInstanceDetail(
	instance connectortools.Instance, tokenChanged, tokenCleared bool,
) map[string]any {
	detail := map[string]any{
		"connector":    instance.Connector,
		"enabled":      instance.Enabled,
		"tokenChanged": tokenChanged,
		"tokenCleared": tokenCleared,
	}
	// The binding, by the fields already classified as safe to return: which
	// vault answers and which role is bound. No mount path beyond the one an
	// operator typed, no token, and nothing a credential could hide in.
	if instance.Connector == "sql" {
		detail["sql"] = map[string]any{
			"host":     instance.SQL.Host,
			"port":     instance.SQL.Port,
			"database": instance.SQL.Database,
			"credentialSource": map[string]any{
				"kind":          string(instance.SQL.CredentialSource.Kind),
				"vaultInstance": instance.SQL.CredentialSource.VaultInstance,
				"mount":         instance.SQL.CredentialSource.Mount,
				"role":          instance.SQL.CredentialSource.Role,
			},
		}
	}
	if instance.Connector == "vault" {
		detail["vault"] = map[string]any{
			"address":             instance.Vault.Address,
			"mount":               instance.Vault.Mount,
			"namespace":           instance.Vault.Namespace,
			"allowedPathPrefixes": len(instance.Vault.AllowedPathPrefixes),
		}
	}
	return detail
}

/*
validateAgainstConfigured judges the instance being written beside the ones
already stored.

The instance replaces its stored self rather than being appended, so editing a
binding is validated as the configuration that will exist and not as one where
the old and new both do.

This does not make the runtime's revalidation optional. Configuration changes
after it is written, and a vault disabled tomorrow must not authorise a query
today because the binding was valid when it was saved.
*/
func (c *ConnectorInstances) validateAgainstConfigured(
	ctx context.Context, instance connectortools.Instance,
	scopeKind settings.ScopeKind, storedScope domain.Scope,
) error {
	stored, err := connectortools.NewSettings(c.settings).Configured(ctx)
	if err != nil {
		return err
	}
	set := []connectortools.Instance{instance}
	for _, other := range stored {
		if sameSetting(other, storedScope, instance.Name) {
			continue
		}
		set = append(set, other.Instance)
	}
	return connectortools.ValidateBindings(set)
}

/*
sameSetting is the row this write replaces, by the key the store actually uses.

Scope kind, scope and name — not the connector. Two instances may share a name
in different scopes, and matching on name alone removed both, so editing an
area's vault dropped the company's vault from the set and let an ambiguous
configuration validate and then be written. The connector is left out on
purpose: a body may change the connector stored under a key, and the row being
replaced is still that key's row.
*/
func sameSetting(
	other connectortools.ConfiguredInstance, storedScope domain.Scope, name string,
) bool {
	if other.Name != name {
		return false
	}
	// The stored scope alone, not the scope kind beside it. The kind is what
	// produces the stored scope, so comparing both puts one rule in two places
	// — and the second copy is the one no sabotage can reach, which is how a
	// weakened rule goes unnoticed.
	_, otherStored, err := connectorScopes(other.ScopeKind, other.Scope)
	if err != nil {
		return false
	}
	return otherStored == storedScope
}
