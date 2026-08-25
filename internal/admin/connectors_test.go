package admin_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

func newConnectorInstances(t *testing.T) (*admin.ConnectorInstances, *settings.Store, *pgxpool.Pool) {
	t.Helper()
	pool := openPool(t)
	if _, err := pool.Exec(context.Background(),
		`delete from settings where kind = 'connector_instance'`); err != nil {
		t.Fatalf("clean settings: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`delete from admin_events where action like 'connector_instance.%'`); err != nil {
		t.Fatalf("clean audit: %v", err)
	}
	key := make([]byte, 32)
	v, err := vault.New(key, "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	store := settings.NewStore(pool, v)
	return admin.NewConnectorInstances(pool, store), store, pool
}

func TestPutConnectorInstance_neverReturnsOrAuditsTheToken(t *testing.T) {
	instances, _, pool := newConnectorInstances(t)
	ctx := context.Background()
	token := "vault-token-that-must-not-leak"

	if err := instances.PutConnectorInstance(ctx, "usr_ana", platform,
		settings.ScopeArea, domain.Scope{Company: "acme", Area: "platform"},
		vaultInstance("prod", true), &token, false); err != nil {
		t.Fatalf("PutConnectorInstance: %v", err)
	}

	listed, err := instances.ConnectorInstances(ctx)
	if err != nil {
		t.Fatalf("ConnectorInstances: %v", err)
	}
	if len(listed) != 1 || !listed[0].HasToken || listed[0].Token != "" {
		t.Fatalf("listed = %+v, want presence without plaintext token", listed)
	}
	var detail string
	if err := pool.QueryRow(ctx,
		`select detail::text from admin_events where target = 'prod' order by event_id desc limit 1`,
	).Scan(&detail); err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if strings.Contains(detail, token) {
		t.Fatalf("audit detail leaked the token: %s", detail)
	}
}

func TestPutConnectorInstance_omittedTokenKeepsTheStoredToken(t *testing.T) {
	instances, store, _ := newConnectorInstances(t)
	ctx := context.Background()
	token := "vault-token"
	instance := vaultInstance("prod", true)
	if err := instances.PutConnectorInstance(ctx, "usr_ana", platform,
		settings.ScopeCompany, domain.Scope{Company: "acme"}, instance, &token, false); err != nil {
		t.Fatalf("PutConnectorInstance: %v", err)
	}

	instance.Vault.Address = "https://vault-b.example"
	if err := instances.PutConnectorInstance(ctx, "usr_ana", platform,
		settings.ScopeCompany, domain.Scope{Company: "acme"}, instance, nil, false); err != nil {
		t.Fatalf("PutConnectorInstance update: %v", err)
	}

	revealed, err := store.Reveal(ctx,
		settings.ScopeCompany, domain.Scope{Company: "acme"},
		settings.KindConnectorInstance, "prod")
	if err != nil {
		t.Fatalf("Reveal: %v", err)
	}
	if revealed.Secret != token {
		t.Fatalf("secret = %q, want the stored token kept", revealed.Secret)
	}
}

func TestPutConnectorInstance_enabledWithoutTokenIsRefused(t *testing.T) {
	instances, _, _ := newConnectorInstances(t)
	err := instances.PutConnectorInstance(context.Background(), "usr_ana", platform,
		settings.ScopeArea, domain.Scope{Company: "acme", Area: "platform"},
		vaultInstance("prod", true), nil, false)
	if !errors.Is(err, admin.ErrConnectorNeedsToken) {
		t.Fatalf("err = %v, want ErrConnectorNeedsToken", err)
	}
}

func TestPutConnectorInstance_cannotClearTokenWhileEnabled(t *testing.T) {
	instances, _, _ := newConnectorInstances(t)
	ctx := context.Background()
	token := "vault-token"
	if err := instances.PutConnectorInstance(ctx, "usr_ana", platform,
		settings.ScopeArea, domain.Scope{Company: "acme", Area: "platform"},
		vaultInstance("prod", true), &token, false); err != nil {
		t.Fatalf("PutConnectorInstance: %v", err)
	}
	err := instances.PutConnectorInstance(ctx, "usr_ana", platform,
		settings.ScopeArea, domain.Scope{Company: "acme", Area: "platform"},
		vaultInstance("prod", true), nil, true)
	if !errors.Is(err, admin.ErrConnectorNeedsToken) {
		t.Fatalf("err = %v, want ErrConnectorNeedsToken", err)
	}
}

func TestDeleteConnectorInstance_removesOnlyTheNamedScope(t *testing.T) {
	instances, _, _ := newConnectorInstances(t)
	ctx := context.Background()
	token := "vault-token"
	for _, scope := range []domain.Scope{
		{Company: "acme", Area: "platform"},
		{Company: "acme", Area: "cx"},
	} {
		if err := instances.PutConnectorInstance(ctx, "usr_ana", platform,
			settings.ScopeArea, scope, vaultInstance("prod", true), &token, false); err != nil {
			t.Fatalf("PutConnectorInstance %s: %v", scope, err)
		}
	}

	if err := instances.DeleteConnectorInstance(ctx, "usr_ana", platform,
		settings.ScopeArea, domain.Scope{Company: "acme", Area: "cx"}, "prod"); err != nil {
		t.Fatalf("DeleteConnectorInstance: %v", err)
	}
	listed, err := instances.ConnectorInstances(ctx)
	if err != nil {
		t.Fatalf("ConnectorInstances: %v", err)
	}
	if len(listed) != 1 || listed[0].Scope.Area != "platform" {
		t.Fatalf("listed = %+v, want only the platform scoped instance", listed)
	}
}

func vaultInstance(name string, enabled bool) connectortools.Instance {
	return connectortools.Instance{
		Name: name, Connector: "vault", Enabled: enabled,
		Vault: connectortools.VaultConfig{
			Address: "https://vault.example", Mount: "secret",
			AllowedPathPrefixes: []string{"certs"},
		},
	}
}
