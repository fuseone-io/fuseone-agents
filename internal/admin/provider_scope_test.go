package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

// newIntegrationsWithStore is newIntegrations with its store, for a test that
// writes a row the doors cannot yet.
func newIntegrationsWithStore(t *testing.T) (*admin.Integrations, *settings.Store) {
	t.Helper()
	pool := openPool(t)
	if _, err := pool.Exec(context.Background(), `delete from settings where kind = 'model_provider'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `delete from settings where kind = 'model_provider'`) })
	store := settings.NewStore(pool, v)
	return admin.NewIntegrations(pool, store), store
}

/*
A listed provider says the runtime scope of its row (NT-019, phase 2 writers,
part B2a), so the reduced answer an author picks from keeps one only where a
run at the target reaches it. Providers are written at the installation
today; a row stored elsewhere says where.
*/
func TestProviders_eachCarriesItsRowsRuntimeScope(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	if err := i.PutProvider(ctx, "usr_ana", platform,
		domain.ModelProvider{Name: "openai", Kind: "openai_compatible", BaseURL: "https://api.openai.com/v1", Enabled: true},
		"sk-secret"); err != nil {
		t.Fatalf("PutProvider: %v", err)
	}
	pool := openPool(t)
	if _, err := pool.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('area', 'acme', 'cx', 'model_provider', 'cx-llm', '{"kind":"openai_compatible"}', true, 'usr_ana')`); err != nil {
		t.Fatalf("plant: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `delete from settings where kind = 'model_provider'`) })

	providers, err := i.Providers(ctx)
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	got := map[string]domain.Scope{}
	for _, p := range providers {
		got[p.Name] = p.Scope
	}
	if got["openai"] != (domain.Scope{Company: domain.Installation}) || got["cx-llm"] != cxScope {
		t.Errorf("providers at %v, want openai at the installation and cx-llm at %s", got, cxScope)
	}
}

// The list a process builds its registry from carries the same scope (phase 3
// readers): the registry finds a provider only from inside it, and one whose
// read lost its scope would reach every run.
func TestProvidersWithCredentials_eachCarriesItsRowsRuntimeScope(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	if err := i.PutProvider(ctx, "usr_ana", platform,
		domain.ModelProvider{Name: "openai", Kind: "openai_compatible", BaseURL: "https://api.openai.com/v1", Enabled: true},
		"sk-secret"); err != nil {
		t.Fatalf("PutProvider: %v", err)
	}
	pool := openPool(t)
	if _, err := pool.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('area', 'acme', 'cx', 'model_provider', 'cx-llm', '{"kind":"openai_compatible"}', false, 'usr_ana')`); err != nil {
		t.Fatalf("plant: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `delete from settings where kind = 'model_provider'`) })

	configured, err := i.ProvidersWithCredentials(ctx)
	if err != nil {
		t.Fatalf("ProvidersWithCredentials: %v", err)
	}
	got := map[string]domain.Scope{}
	for _, p := range configured {
		got[p.Name] = p.Scope
	}
	if got["openai"] != (domain.Scope{Company: domain.Installation}) || got["cx-llm"] != cxScope {
		t.Errorf("configured at %v, want openai at the installation and cx-llm (switched off) at %s", got, cxScope)
	}
}

// Read by name wherever it is stored, its key opened under its own scope.
func TestProviderNamed_anAreasProvider_isReadAtItsOwnScope(t *testing.T) {
	i, store := newIntegrationsWithStore(t)
	ctx := context.Background()
	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeArea, Scope: cxScope, Kind: settings.KindModelProvider, Name: "cx-llm",
		Value: []byte(`{"kind":"openai_compatible","baseURL":"https://cx.example.com/v1"}`), Secret: "sk-cx", Enabled: true, UpdatedBy: "usr_ana",
	}); err != nil {
		t.Fatalf("store the area's provider: %v", err)
	}
	provider, err := i.ProviderNamed(ctx, "cx-llm")
	if err != nil || provider.Scope != cxScope || provider.SecretFingerprint == "" {
		t.Fatalf("ProviderNamed = %+v, %v; want acme/cx's provider with its key fingerprinted", provider, err)
	}
	if key, err := i.Credential(ctx, "cx-llm"); err != nil || key != "sk-cx" {
		t.Fatalf("Credential = %q, %v; want the key sealed at acme/cx", key, err)
	}
}

// A row whose columns contradict its kind is no scope: every reader refuses it,
// and the registry's list holds its name with no scope, so the environment
// does not fill it either.
func TestProviderReaders_aRowOfNoKnownScope_isRefusedByEveryReader(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	pool := openPool(t)
	if _, err := pool.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('company', 'acme', 'cx', 'model_provider', 'openai', '{"kind":"openai_compatible","baseURL":"https://x.example.com/v1"}', true, 'usr_ana')`); err != nil {
		t.Fatalf("plant: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `delete from settings where kind = 'model_provider'`) })

	listed, err := i.Providers(ctx)
	if err != nil || len(listed) != 0 {
		t.Errorf("Providers = %+v, %v; want the row left out", listed, err)
	}
	if _, err := i.ProviderNamed(ctx, "openai"); !errors.Is(err, admin.ErrProviderUnknown) {
		t.Errorf("ProviderNamed = %v, want unknown", err)
	}
	configured, err := i.ProvidersWithCredentials(ctx)
	if err != nil || len(configured) != 1 || configured[0].Scope != (domain.Scope{}) {
		t.Errorf("ProvidersWithCredentials = %+v, %v; want openai held with no scope", configured, err)
	}
}

// The scope is judged before the value is read: a row a later release writes
// in a shape this binary cannot decode must not stop the refresh, or the
// provider it replaced would stay in the registry and the environment keep its
// name.
func TestProviderReaders_aRowOfNoKnownScope_isRefusedBeforeItsValueIsRead(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	pool := openPool(t)
	if _, err := pool.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('company', 'acme', 'cx', 'model_provider', 'future', '"a shape this binary cannot read"', true, 'usr_ana')`); err != nil {
		t.Fatalf("plant: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `delete from settings where kind = 'model_provider'`) })

	if listed, err := i.Providers(ctx); err != nil || len(listed) != 0 {
		t.Errorf("Providers = %+v, %v; want the row left out without reading it", listed, err)
	}
	configured, err := i.ProvidersWithCredentials(ctx)
	if err != nil || len(configured) != 1 || configured[0].Name != "future" || configured[0].Scope != (domain.Scope{}) {
		t.Errorf("ProvidersWithCredentials = %+v, %v; want future held by name with no scope", configured, err)
	}
}
