package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

/*
The scope of a configured server reaches the catalogue (NT-019, phase 2 readers).

Through the real connection: the row's scope rides on the server the
administration reads, and connectServer hands it to the catalogue, where the
Gate reads it. A connection that dropped it would make an area's server the
installation's, reaching every run.
*/
func TestConnectServer_anAreasServer_isItsAreasInTheCatalogue(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "scoped")
			respondRPC(t, w, req.ID, map[string]any{
				"protocolVersion": "2025-11-25", "capabilities": map[string]any{},
				"serverInfo": map[string]any{"name": "jira", "version": "test"},
			})
		case "tools/list":
			respondRPC(t, w, req.ID, map[string]any{"tools": []map[string]any{{
				"name": "search", "description": "Search issues.", "inputSchema": map[string]any{"type": "object"},
			}}})
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	t.Cleanup(remote.Close)

	cx := domain.Scope{Company: "acme", Area: "cx"}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	if err := connectServer(t.Context(), catalog, domain.MCPServer{
		Name: "jira", Transport: domain.TransportHTTP, URL: remote.URL,
		ProtocolMode: domain.MCPProtocolLegacy, Enabled: true, Scope: cx,
	}, domain.MCPCredentials{}, nil, nil, credentialPolicy{}, nil); err != nil {
		t.Fatalf("connectServer: %v", err)
	}
	t.Cleanup(func() { _ = catalog.RemoveServer("jira") })

	if got, ok := catalog.Scope("jira.search"); !ok || got != cx {
		t.Errorf("scope = %s (%v), want %s", got, ok, cx)
	}
}
