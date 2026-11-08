package admin

import (
	"context"
	"errors"
	"fmt"

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
	_, err := c.WriteConnectorInstance(ctx, ConnectorWrite{
		By: by, ScopeKind: scopeKind, Scope: scope, Instance: instance, Token: token, ClearToken: clearToken,
	})
	return err
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
	if instance.Connector == "elasticsearch" {
		detail["elasticsearch"] = map[string]any{
			"baseUrl":       instance.Elasticsearch.BaseURL,
			"username":      instance.Elasticsearch.Username,
			"index":         instance.Elasticsearch.Index,
			"maxWindowDays": instance.Elasticsearch.MaxWindowDays,
		}
	}
	if instance.Connector == "evidence" {
		detail["evidence"] = map[string]any{
			"baseUrl": instance.Evidence.BaseURL,
			"queries": len(instance.Evidence.Queries),
		}
	}
	if instance.Connector == "cloudflare" {
		detail["cloudflare"] = map[string]any{
			"accountId":       instance.Cloudflare.AccountID,
			"listId":          instance.Cloudflare.ListID,
			"protectedRanges": len(instance.Cloudflare.ProtectedRanges),
			"maxBlocksPerDay": instance.Cloudflare.MaxBlocksPerDay,
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
