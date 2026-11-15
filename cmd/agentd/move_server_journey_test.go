package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

/*
Moving a server out of the installation, from the HTTP request to the row
(NT-019): the If-Match the client read travels the generated door into the
store's precondition, and the server reads back at the area it moved to.
*/
func TestMoveMCPServer_fromTheRequest_storesTheServerAtTheArea(t *testing.T) {
	pool := testdb.Pool(t)
	w := readBackWorldOn(t, pool)
	const path = "/admin/integrations/mcp-servers/move-journey"
	if _, err := pool.Exec(t.Context(), `
		delete from settings where kind = 'mcp_server' and name = 'move-journey';
		insert into companies (company_id, name) values ('acme', 'Acme') on conflict do nothing;
		insert into scopes (company_id, area_id, label) values ('acme', 'cx', 'CX') on conflict do nothing`); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := admin.NewIntegrations(pool, w.store).PutMCPEgressPolicy(t.Context(), "usr_ana",
		domain.Scope{Company: "default", Area: "platform"},
		admin.MCPEgressPolicy{Scope: domain.Scope{Company: "acme", Area: "cx"}, Origins: []string{"https://mcp.example.com"}}); err != nil {
		t.Fatalf("PutMCPEgressPolicy: %v", err)
	}
	put := w.call(t, etagCall{method: http.MethodPut, path: path,
		body: map[string]any{"transport": "http", "url": "https://mcp.example.com/m", "token": "tok-journey"}})
	if put.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d: %s", put.Code, put.Body)
	}

	if got := w.call(t, etagCall{method: http.MethodPost, path: path + "/move",
		body: map[string]any{"company": "acme", "area": "cx"}}); got.Code != http.StatusPreconditionRequired {
		t.Fatalf("move without If-Match = %d: %s", got.Code, got.Body)
	}
	moved := w.call(t, etagCall{method: http.MethodPost, path: path + "/move",
		ifMatch: []string{put.Header().Get("ETag")}, body: map[string]any{"company": "acme", "area": "cx"}})
	if moved.Code != http.StatusNoContent || moved.Header().Get("ETag") == "" {
		t.Fatalf("move = %d with ETag %q: %s", moved.Code, moved.Header().Get("ETag"), moved.Body)
	}

	read := w.call(t, etagCall{method: http.MethodGet, path: path})
	var got struct {
		ScopeKind, Company, Area string
	}
	if err := json.Unmarshal(read.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %s: %v", read.Body, err)
	}
	if read.Code != http.StatusOK || got.ScopeKind != "area" || got.Company != "acme" || got.Area != "cx" {
		t.Fatalf("read back = %d %+v, want the server at acme/cx", read.Code, got)
	}
	if read.Header().Get("ETag") != moved.Header().Get("ETag") {
		t.Errorf("read ETag %q, want the one the move answered %q", read.Header().Get("ETag"), moved.Header().Get("ETag"))
	}
}
