package connectortools

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/vault"
)

func TestGraviteeAccessResolver_readsTheStoredBindingAsOneAuthority(t *testing.T) {
	pool := graviteeTestPool(t)
	store := graviteeSettingsStore(t, pool)
	scope := area("gravitee-authority-test", "runtime")
	writeConnectorSetting(t, store, settings.ScopeArea, scope,
		graviteeInstance(scope, graviteeSource("secrets")), "")
	writeConnectorSetting(t, store, settings.ScopeArea, scope,
		graviteeVault("secrets", scope), "vault-connector-token")
	secrets := &secretFieldSpy{value: SecretValue{value: "gravitee-token"}}

	access, err := NewGraviteeAccessResolver(NewSettings(store), secrets).
		Resolve(t.Context(), "apim", scope)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if access.Config.Organization != "org-prod" || access.credential.value != "gravitee-token" ||
		secrets.path != "integrations/gravitee/prod" || secrets.token != "vault-connector-token" {
		t.Fatalf("access = %+v, secret read = %+v", access.Config, secrets)
	}

	// A restore can bypass the administrative validation. The runtime owes the
	// same boundary before it opens either credential.
	invalid := graviteeInstance(scope, graviteeSource("secrets"))
	invalid.Gravitee.CredentialSource.Path = "another/team/token"
	writeConnectorSetting(t, store, settings.ScopeArea, scope, invalid, "")
	secrets.calls = 0
	if _, err := NewGraviteeAccessResolver(NewSettings(store), secrets).
		Resolve(t.Context(), "apim", scope); err == nil {
		t.Fatal("restored out-of-policy binding was accepted")
	}
	if secrets.calls != 0 {
		t.Fatalf("secret reads = %d, want none before the restored binding passes", secrets.calls)
	}
}

func graviteeTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.Pool(t)
	if _, err := pool.Exec(t.Context(),
		`delete from settings where kind = 'connector_instance' and company_id = 'gravitee-authority-test'`); err != nil {
		t.Fatalf("clean connector settings: %v", err)
	}
	return pool
}

func graviteeSettingsStore(t *testing.T, pool *pgxpool.Pool) *settings.Store {
	t.Helper()
	sealed, err := vault.New(make([]byte, 32), "gravitee-authority-test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return settings.NewStore(pool, sealed)
}

func writeConnectorSetting(
	t *testing.T, store *settings.Store, scopeKind settings.ScopeKind,
	scope domain.Scope, instance Instance, secret string,
) {
	t.Helper()
	value, err := SettingValue(instance)
	if err != nil {
		t.Fatalf("SettingValue: %v", err)
	}
	if err := store.Put(t.Context(), settings.Setting{
		ScopeKind: scopeKind, Scope: scope, Kind: settings.KindConnectorInstance,
		Name: instance.Name, Value: value, Secret: secret, Enabled: instance.Enabled,
		UpdatedBy: "usr_test",
	}); err != nil {
		t.Fatalf("Put %s: %v", instance.Name, err)
	}
}

type secretFieldSpy struct {
	value SecretValue
	calls int
	token string
	path  string
}

func (s *secretFieldSpy) ReadSecretField(
	_ context.Context, _ VaultConfig, token, path, _ string,
) (SecretValue, error) {
	s.calls++
	s.token, s.path = token, path
	return s.value, nil
}
