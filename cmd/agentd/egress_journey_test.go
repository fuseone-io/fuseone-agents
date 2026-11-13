package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/tools"
)

/*
From the platform's door to the worker's connection (NT-019).

The policy is put through the API as the console would, the area's server is
a stored row, and the worker reads both through the real administration. A
field that dies in a middle layer would leave the worker connecting the area's
server to anything, or to nothing whatever the policy said.
*/
func TestEgressJourney_aPolicyPutThroughTheAPI_holdsTheWorkersAreaServer(t *testing.T) {
	pool := testdb.Pool(t)
	w := readBackWorldOn(t, pool)
	ctx := context.WithoutCancel(t.Context())
	remote := httptest.NewServer(mcpHandler())
	t.Cleanup(remote.Close)
	area := domain.Scope{Company: "acme", Area: "egress-journey"}

	value, err := json.Marshal(map[string]any{"transport": "http", "url": remote.URL})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := w.store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeArea, Scope: area, Kind: settings.KindMCPServer, Name: "egress-journey",
		Value: value, Enabled: true, UpdatedBy: "usr_ana",
	}); err != nil {
		t.Fatalf("store the area's server: %v", err)
	}
	t.Cleanup(func() {
		_ = w.store.Delete(ctx, settings.ScopeArea, area, settings.KindMCPServer, "egress-journey")
		w.send(t, http.MethodDelete, "/admin/integrations/mcp-egress-policies?company=acme&area=egress-journey", nil)
	})

	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	r := newReconciler(catalog, admin.NewIntegrations(pool, w.store), recordingHealth{})
	r.reconcile(ctx)
	if catalog.Connected("egress-journey") {
		t.Fatal("the area's server connected with no policy for its scope")
	}

	w.put(t, "/admin/integrations/mcp-egress-policies", map[string]any{
		"company": "acme", "area": "egress-journey", "origins": []string{remote.URL},
	})
	r.reconcile(ctx)
	if !catalog.Connected("egress-journey") {
		t.Error("the area's server did not connect once its origin was allowed")
	}
}

/*
A policy's origins are said, never assumed (NT-019).

`origins: []` is a deny-all, and replaces whatever the scope had. A body that
forgets the field, or sends null, says nothing: taken as a deny-all it would
replace a working list by accident and cut the scope's servers off.
*/
func TestEgressJourney_originsAbsentOrNull_areRefused_andAnEmptyListIsADenyAll(t *testing.T) {
	pool := testdb.Pool(t)
	w := readBackWorldOn(t, pool)
	const path = "/admin/integrations/mcp-egress-policies"
	t.Cleanup(func() { w.send(t, http.MethodDelete, path+"?company=acme&area=egress-origins", nil) })

	for name, body := range map[string]map[string]any{
		"absent": {"company": "acme", "area": "egress-origins"},
		"null":   {"company": "acme", "area": "egress-origins", "origins": nil},
	} {
		if code, answer := w.send(t, http.MethodPut, path, body); code != http.StatusBadRequest {
			t.Errorf("origins %s: %d %s, want 400", name, code, answer)
		}
	}
	if code, answer := w.send(t, http.MethodPut, path, map[string]any{
		"company": "acme", "area": "egress-origins", "origins": []string{},
	}); code != http.StatusNoContent {
		t.Errorf("origins []: %d %s, want 204", code, answer)
	}
}
