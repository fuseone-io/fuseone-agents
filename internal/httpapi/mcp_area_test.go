package httpapi

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/vault"
)

/*
An area configures its own remote MCP servers (NT-019, phase 2 writers).

Journeyed through the real store: the door authorizes at the server's scope,
refuses a local server from an area (decided 2026-11-13), and the listing shows
an area what it owns and inherits, never a sibling's.
*/

func mcpWorld(t *testing.T) *Server {
	t.Helper()
	pool := testdb.Isolated(t)
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return NewServer(ledger.NewMemory(), "test").
		WithAdministration(nil, nil, admin.NewIntegrations(pool, settings.NewStore(pool, v)))
}

func putMCP(t *testing.T, s *Server, caller context.Context, name string, at target, local bool) openapi.PutMCPServerResponseObject {
	t.Helper()
	body := openapi.PutMCPServerJSONRequestBody{
		ScopeKind: ptr(at.kind), Company: at.company, Area: at.area,
		Transport: ptr(openapi.TransportHttp), Url: ptr("https://mcp.example.com/" + name),
	}
	if local {
		body.Transport, body.Url = ptr(openapi.TransportStdio), nil
		body.Command, body.AcceptsLocalExecution = ptr("sh"), ptr(true)
	}
	resp, err := s.PutMCPServer(caller, openapi.PutMCPServerRequestObject{Name: name, Body: &body})
	if err != nil {
		t.Fatalf("PutMCPServer %s: %v", name, err)
	}
	return resp
}

func TestPutMCPServer_isAuthorizedAtTheServersScope_andAnAreasIsRemote(t *testing.T) {
	s := mcpWorld(t)
	for _, one := range []struct {
		name   string
		caller context.Context
		at     target
		local  bool
		want   string
	}{
		{"cx-jira", grantedAt(domain.RoleAdmin, cx), atCX, false, "204"},
		{"cx-local", grantedAt(domain.RoleAdmin, cx), atCX, true, "400"},
		{"fin-jira", grantedAt(domain.RoleAdmin, cx), atFinance, false, "403"},
		{"platform-jira", grantedAt(domain.RoleAdmin, cx), atInstallation, false, "403"},
		{"acme-jira", grantedAt(domain.RoleAdmin, acmeOnly), atAcme, false, "204"},
		{"gov-jira", as(domain.RoleCurator), atFinance, false, "204"},
		{"platform-local", as(domain.RoleCurator), atInstallation, true, "204"},
		{"cx-jira", as(domain.RoleCurator), atFinance, false, "409"},
	} {
		got := ""
		switch putMCP(t, s, one.caller, one.name, one.at, one.local).(type) {
		case openapi.PutMCPServer204Response:
			got = "204"
		case openapi.PutMCPServer400ApplicationProblemPlusJSONResponse:
			got = "400"
		case openapi.PutMCPServer403ApplicationProblemPlusJSONResponse:
			got = "403"
		case openapi.PutMCPServer409ApplicationProblemPlusJSONResponse:
			got = "409"
		}
		if got != one.want {
			t.Errorf("%s at %v: got %q, want %s", one.name, one.at.kind, got, one.want)
		}
	}
}

func TestMCPServerDoorsByName_areAuthorizedAtTheServersScope(t *testing.T) {
	s := mcpWorld(t)
	putMCP(t, s, as(domain.RoleCurator), "cx-jira", atCX, false)
	putMCP(t, s, as(domain.RoleCurator), "fin-jira", atFinance, false)
	areaAdmin := grantedAt(domain.RoleAdmin, cx)

	for name, refused := range map[string]bool{"cx-jira": false, "fin-jira": true, "nobody-has-this": true} {
		got, err := s.GetMCPServer(areaAdmin, openapi.GetMCPServerRequestObject{Name: name})
		if err != nil {
			t.Fatalf("get %s: %v", name, err)
		}
		if _, is403 := got.(openapi.GetMCPServer403ApplicationProblemPlusJSONResponse); is403 != refused {
			t.Errorf("get %s = %T, want refused %v", name, got, refused)
		}
		probe, err := s.ProbeMCPServer(areaAdmin, openapi.ProbeMCPServerRequestObject{Name: name})
		if err != nil {
			t.Fatalf("probe %s: %v", name, err)
		}
		if _, is403 := probe.(openapi.ProbeMCPServer403ApplicationProblemPlusJSONResponse); is403 != refused {
			t.Errorf("probe %s = %T, want refused %v", name, probe, refused)
		}
	}
	gone, err := s.DeleteMCPServer(areaAdmin, openapi.DeleteMCPServerRequestObject{Name: "fin-jira"})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := gone.(openapi.DeleteMCPServer403ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("deleting a sibling's server = %T, want 403", gone)
	}
}

func TestListIntegrations_showsAnAreaItsServersAndTheInherited(t *testing.T) {
	s := mcpWorld(t)
	putMCP(t, s, as(domain.RoleCurator), "platform-jira", atInstallation, false)
	putMCP(t, s, as(domain.RoleCurator), "cx-jira", atCX, false)
	putMCP(t, s, as(domain.RoleCurator), "fin-jira", atFinance, false)

	resp, err := s.ListIntegrations(grantedAt(domain.RoleAdmin, cx), openapi.ListIntegrationsRequestObject{})
	if err != nil {
		t.Fatalf("ListIntegrations: %v", err)
	}
	list, ok := resp.(openapi.ListIntegrations200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", resp)
	}
	editable := map[string]bool{}
	for _, srv := range list.McpServers {
		editable[srv.Name] = srv.Editable != nil && *srv.Editable
	}
	if _, sibling := editable["fin-jira"]; sibling {
		t.Error("an area was shown a sibling area's server")
	}
	if !editable["cx-jira"] {
		t.Errorf("own server editable = %v, want true", editable["cx-jira"])
	}
	if e, inherited := editable["platform-jira"]; !inherited || e {
		t.Errorf("platform server listed %v editable %v, want listed and not editable", inherited, e)
	}
}

// An author configures nothing, and the integrations listing is not theirs.
func TestListIntegrations_anAreaAuthor_isRefused(t *testing.T) {
	s := mcpWorld(t)
	resp, err := s.ListIntegrations(grantedAt(domain.RoleAuthor, cx), openapi.ListIntegrationsRequestObject{})
	if err != nil {
		t.Fatalf("ListIntegrations: %v", err)
	}
	if _, ok := resp.(openapi.ListIntegrations403ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T, want 403", resp)
	}
}
