package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

// ConnectorWrite is an instance being configured, and the precondition it answers.
type ConnectorWrite struct {
	By         domain.UserID
	ScopeKind  settings.ScopeKind
	Scope      domain.Scope
	Instance   connectortools.Instance
	Token      *string
	ClearToken bool
	Expect     domain.IfMatch
}

// connectorRevised names an instance by its declaration and its token. Workers read these
// whole on a timer and compare nothing, so no write has an effect but the one it states;
// and the write does not need the stored token, so a lost one can be replaced.
func (c *ConnectorInstances) connectorRevised(expect domain.IfMatch) *revised {
	return &revised{
		expect:             expect,
		of:                 func(set settings.Setting) domain.Revision { return c.settings.Revision(set, []byte(set.Secret), nil) },
		ofUnreadable:       func(set settings.Setting) domain.Revision { return c.settings.RevisionUnreadable(set, nil) },
		replacesUnreadable: true,
	}
}

// ConnectorInstanceRevision names one stored instance. A token that will not open still
// has a revision: reading a declaration has never needed the token.
func (c *ConnectorInstances) ConnectorInstanceRevision(
	ctx context.Context, scopeKind settings.ScopeKind, scope domain.Scope, name string,
) (domain.Revision, error) {
	storedScope, _, err := connectorScopes(scopeKind, scope)
	if err != nil {
		return "", err
	}
	named := c.connectorRevised(domain.IfMatch{})
	kind := settings.KindConnectorInstance
	row, err := c.settings.Reveal(ctx, scopeKind, storedScope, kind, name)
	if errors.Is(err, vault.ErrCiphertext) || errors.Is(err, settings.ErrNoVault) {
		held, err := c.settings.Get(ctx, scopeKind, storedScope, kind, name)
		if err != nil {
			return "", err
		}
		return named.ofUnreadable(held), nil
	}
	if err != nil {
		return "", err
	}
	return named.of(row), nil
}

// WriteConnectorInstance stores an instance and answers its resulting revision.
func (c *ConnectorInstances) WriteConnectorInstance(ctx context.Context, w ConnectorWrite) (domain.Revision, error) {
	by, scopeKind, scope, instance, token, clearToken := w.By, w.ScopeKind, w.Scope, w.Instance, w.Token, w.ClearToken
	storedScope, runtimeScope, err := connectorScopes(scopeKind, scope)
	if err != nil {
		return "", err
	}
	instance.Scope = runtimeScope
	needsToken := connectortools.RequiresToken(instance.Connector)
	if !needsToken && token != nil && strings.TrimSpace(*token) != "" {
		return "", fmt.Errorf("admin: connector %s must not carry a token; its authority comes from its binding",
			instance.Connector)
	}
	if needsToken && instance.Enabled && token != nil && strings.TrimSpace(*token) == "" {
		return "", ErrConnectorNeedsToken
	}
	if err := connectortools.ValidateInstanceConfig(instance); err != nil {
		return "", err
	}
	// A binding names another instance, so it can only be judged against the
	// set. Reads settings, not Vault: saving a configuration must not reach the
	// system it configures, or an operator cannot write down an intention
	// before the thing it points at is reachable.
	if err := c.validateAgainstConfigured(ctx, instance, scopeKind, storedScope); err != nil {
		return "", err
	}
	value, err := connectortools.SettingValue(instance)
	if err != nil {
		return "", err
	}
	secret := ""
	if token != nil {
		secret = *token
	}
	base := settings.Setting{
		ScopeKind: scopeKind, Scope: storedScope,
		Kind: settings.KindConnectorInstance, Name: instance.Name,
		Value: value, Secret: secret,
		Enabled: instance.Enabled, UpdatedBy: string(by),
	}
	return writeRevised(ctx, c.pool, c.settings, folded{
		by: by, scope: runtimeScope, action: "connector_instance.configured", target: instance.Name,
		set: base, revised: c.connectorRevised(w.Expect),
		fold: func(stored settings.Setting) (settings.Setting, any, error) {
			// A bound connector never has a secret of its own. Clearing here also
			// removes one left by an older writer when the instance is next edited.
			clear := (!needsToken && stored.HasSecret) || (clearToken && secret == "")
			willHaveSecret := secret != "" || (!clear && stored.HasSecret)
			if needsToken && instance.Enabled && !willHaveSecret {
				return settings.Setting{}, nil, ErrConnectorNeedsToken
			}
			set := base
			set.ClearSecret = clear
			return set, connectorInstanceDetail(instance, token != nil, clear), nil
		},
	})
}

func (c *ConnectorInstances) DeleteConnectorInstance(
	ctx context.Context, by domain.UserID, _ domain.Scope,
	scopeKind settings.ScopeKind, scope domain.Scope, name string,
) error {
	return c.RemoveConnectorInstance(ctx, ConnectorRemoval{By: by, ScopeKind: scopeKind, Scope: scope, Name: name})
}

// ConnectorRemoval is an instance going, and the precondition it answers.
type ConnectorRemoval struct {
	By        domain.UserID
	ScopeKind settings.ScopeKind
	Scope     domain.Scope
	Name      string
	Expect    domain.IfMatch
}

// RemoveConnectorInstance deletes an instance, if it is still at a revision the caller named.
func (c *ConnectorInstances) RemoveConnectorInstance(ctx context.Context, r ConnectorRemoval) error {
	by, scopeKind, scope, name := r.By, r.ScopeKind, r.Scope, r.Name
	if !connectortools.ValidInstanceName(name) {
		return fmt.Errorf("admin: invalid connector instance name %q", name)
	}
	storedScope, runtimeScope, err := connectorScopes(scopeKind, scope)
	if err != nil {
		return err
	}
	return removeRevised(ctx, c.pool, c.settings, removal{
		by: by, at: scopeKind, stored: storedScope, recorded: runtimeScope,
		kind: settings.KindConnectorInstance, name: name, action: "connector_instance.removed",
		revised: c.connectorRevised(r.Expect),
	})
}
