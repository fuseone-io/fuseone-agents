package admin_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

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
