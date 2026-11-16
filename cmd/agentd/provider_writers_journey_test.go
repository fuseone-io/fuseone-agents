package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

/*
An area's administrator configures the area's own model provider, from the
request to the row (NT-019, phase 3 writers): stored at the area, read back at
the area, listed to the area as editable, invisible to a sibling, and refused a
name another scope holds.
*/

var (
	journeyCX  = domain.Scope{Company: "acme", Area: "cx"}
	journeyOps = domain.Scope{Company: "acme", Area: "ops"}
)

func providerBody(scope map[string]any) map[string]any {
	body := map[string]any{"kind": "openai_compatible", "baseUrl": "https://llm.example.com/v1", "apiKey": "sk-area", "enabled": true}
	for k, v := range scope {
		body[k] = v
	}
	return body
}

func TestProviderWriters_anAreasAdministrator_configuresTheAreasOwnProvider(t *testing.T) {
	pool := testdb.Pool(t)
	w := readBackWorldOn(t, pool)
	if _, err := pool.Exec(t.Context(), `delete from settings where kind = 'model_provider'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(t.Context(), `delete from settings where kind = 'model_provider'`) })
	const path = "/admin/integrations/providers/cx-llm"
	area := map[string]any{"scopeKind": "area", "company": "acme", "area": "cx"}

	if got := w.call(t, etagCall{method: http.MethodPut, path: path, at: journeyCX, body: providerBody(area)}); got.Code != http.StatusNoContent {
		t.Fatalf("PUT at the area = %d: %s", got.Code, got.Body)
	}
	read := w.call(t, etagCall{method: http.MethodGet, path: path, at: journeyCX})
	var got struct{ ScopeKind, Company, Area string }
	_ = json.Unmarshal(read.Body.Bytes(), &got)
	if read.Code != http.StatusOK || got.ScopeKind != "area" || got.Company != "acme" || got.Area != "cx" {
		t.Fatalf("GET = %d %+v: %s", read.Code, got, read.Body)
	}
	if sibling := w.call(t, etagCall{method: http.MethodGet, path: path, at: journeyOps}); sibling.Code != http.StatusForbidden {
		t.Errorf("GET from acme/ops = %d, want refused", sibling.Code)
	}
	if got := w.call(t, etagCall{method: http.MethodDelete, path: path, at: journeyCX}); got.Code != http.StatusNoContent {
		t.Fatalf("DELETE at the area = %d: %s", got.Code, got.Body)
	}
}

func TestProviderWriters_anAreasAdministrator_isRefusedTheInstallationAndATakenName(t *testing.T) {
	pool := testdb.Pool(t)
	w := readBackWorldOn(t, pool)
	if _, err := pool.Exec(t.Context(), `delete from settings where kind = 'model_provider'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(t.Context(), `delete from settings where kind = 'model_provider'`) })

	if got := w.call(t, etagCall{method: http.MethodPut, path: "/admin/integrations/providers/mine", at: journeyCX,
		body: providerBody(nil)}); got.Code != http.StatusForbidden {
		t.Errorf("PUT at the installation by an area = %d, want 403", got.Code)
	}
	if got := w.call(t, etagCall{method: http.MethodPut, path: "/admin/integrations/providers/shared",
		body: providerBody(nil)}); got.Code != http.StatusNoContent {
		t.Fatalf("PUT shared by the platform = %d: %s", got.Code, got.Body)
	}
	area := map[string]any{"scopeKind": "area", "company": "acme", "area": "cx"}
	if got := w.call(t, etagCall{method: http.MethodPut, path: "/admin/integrations/providers/shared", at: journeyCX,
		body: providerBody(area)}); got.Code != http.StatusConflict {
		t.Errorf("PUT shared at the area = %d: %s, want 409", got.Code, got.Body)
	}
}

// The listing: the area's own provider is editable to the area, the
// installation's is listed read-only, and a sibling area sees neither its own
// nor the other's.
func TestProviderWriters_theListing_marksWhatTheCallerMayChange(t *testing.T) {
	pool := testdb.Pool(t)
	w := readBackWorldOn(t, pool)
	if _, err := pool.Exec(t.Context(), `delete from settings where kind = 'model_provider'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(t.Context(), `delete from settings where kind = 'model_provider'`) })
	w.call(t, etagCall{method: http.MethodPut, path: "/admin/integrations/providers/shared", body: providerBody(nil)})
	w.call(t, etagCall{method: http.MethodPut, path: "/admin/integrations/providers/cx-llm", at: journeyCX,
		body: providerBody(map[string]any{"scopeKind": "area", "company": "acme", "area": "cx"})})

	listed := func(at domain.Scope) map[string]bool {
		rec := w.call(t, etagCall{method: http.MethodGet, path: "/admin/integrations", at: at})
		var body struct {
			Providers []struct {
				Name     string
				Editable *bool
			}
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		out := map[string]bool{}
		for _, p := range body.Providers {
			out[p.Name] = p.Editable != nil && *p.Editable
		}
		return out
	}
	if got := listed(journeyCX); len(got) != 2 || !got["cx-llm"] || got["shared"] {
		t.Errorf("acme/cx lists %v, want cx-llm editable and shared read-only", got)
	}
	if got := listed(journeyOps); len(got) != 1 || got["shared"] {
		t.Errorf("acme/ops lists %v, want shared alone, read-only", got)
	}
}
