package httpapi

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

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
	integrations := admin.NewIntegrations(pool, settings.NewStore(pool, v))
	allowAcmeEgress(t, integrations)
	return NewServer(ledger.NewMemory(), "test").WithAdministration(nil, nil, integrations)
}

// allowAcmeEgress lets acme's areas reach the servers these tests write
// (NT-019): the egress policy is not what they are about.
func allowAcmeEgress(t *testing.T, integrations *admin.Integrations) {
	t.Helper()
	if err := integrations.PutMCPEgressPolicy(context.Background(), "usr_ana", adminScope, admin.MCPEgressPolicy{
		Scope: domain.Scope{Company: "acme"}, Origins: []string{"https://mcp.example.com"},
	}); err != nil {
		t.Fatalf("PutMCPEgressPolicy: %v", err)
	}
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

// swapAfterLookup answers where a server is, and then, before the door acts,
// removes it and stores a server of the same name in a sibling area: the race
// a door that looks the name up twice loses.
type swapAfterLookup struct {
	*admin.Integrations
	t       *testing.T
	pool    *pgxpool.Pool
	swapped bool
}

func (w *swapAfterLookup) MCPServerStoredAt(ctx context.Context, name string) (settings.ScopeKind, domain.Scope, error) {
	kind, stored, err := w.Integrations.MCPServerStoredAt(ctx, name)
	if err != nil || w.swapped {
		return kind, stored, err
	}
	w.swapped = true
	if err := w.Integrations.RemoveMCPServer(ctx, admin.Removal{By: "usr_ana", Scope: adminScope, Name: name}); err != nil {
		w.t.Fatalf("remove the area's server: %v", err)
	}
	if _, err := w.Integrations.WriteMCPServer(ctx, admin.MCPServerWrite{
		By: "usr_fin", Scope: finance, At: settings.ScopeArea, Stored: finance,
		Server: domain.MCPServer{Name: name, Transport: domain.TransportHTTP, URL: "https://mcp.example.com/fin", Enabled: true},
	}); err != nil {
		w.t.Fatalf("store the sibling's server: %v", err)
	}
	// And the sibling's tools, as its worker publishes them. A removal
	// authorized for the first server must not take them.
	if _, err := w.pool.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('installation', '', '', 'tool', 'jira.search', '{"server":"jira","onSurface":true}', true, 'w')`); err != nil {
		w.t.Fatalf("publish the sibling's tool: %v", err)
	}
	return kind, stored, nil
}

/*
A door acts on the row it authorized, and no other (NT-019).

Authorized for acme/cx, a DELETE, a probe and a rediscovery must not reach the
server acme/finance stored under the same name in between.
*/
func TestMCPServerDoors_actOnlyOnTheRowTheyAuthorized(t *testing.T) {
	for _, door := range []string{"delete", "probe", "rediscover"} {
		t.Run(door, func(t *testing.T) {
			pool := testdb.Isolated(t)
			v, err := vault.New(make([]byte, 32), "test")
			if err != nil {
				t.Fatalf("vault: %v", err)
			}
			integrations := admin.NewIntegrations(pool, settings.NewStore(pool, v))
			allowAcmeEgress(t, integrations)
			if _, err := integrations.WriteMCPServer(context.Background(), admin.MCPServerWrite{
				By: "usr_cx", Scope: cx, At: settings.ScopeArea, Stored: cx,
				Server: domain.MCPServer{Name: "jira", Transport: domain.TransportHTTP, URL: "https://mcp.example.com/cx", Enabled: true},
			}); err != nil {
				t.Fatalf("the area's server: %v", err)
			}
			s := NewServer(ledger.NewMemory(), "test").
				WithAdministration(nil, nil, &swapAfterLookup{Integrations: integrations, t: t, pool: pool})
			caller := grantedAt(domain.RoleAdmin, cx)
			switch door {
			case "delete":
				_, err = s.DeleteMCPServer(caller, openapi.DeleteMCPServerRequestObject{Name: "jira"})
			case "probe":
				_, err = s.ProbeMCPServer(caller, openapi.ProbeMCPServerRequestObject{Name: "jira"})
			case "rediscover":
				_, err = s.RequestMCPRediscovery(caller, openapi.RequestMCPRediscoveryRequestObject{Name: "jira"})
			}
			if err != nil {
				t.Fatalf("%s: %v", door, err)
			}
			var probes, rediscoveries, servers, tools int
			if err := pool.QueryRow(context.Background(), `
				select (select count(*) from settings where kind = 'mcp_probe'),
				       (select count(*) from mcp_rediscoveries),
				       (select count(*) from settings where kind = 'mcp_server' and area_id = 'finance'),
				       (select count(*) from settings where kind = 'tool' and name = 'jira.search')`,
			).Scan(&probes, &rediscoveries, &servers, &tools); err != nil {
				t.Fatalf("count: %v", err)
			}
			if servers != 1 || tools != 1 || probes != 0 || rediscoveries != 0 {
				t.Errorf("finance's server %d, its tools %d, probes %d, rediscoveries %d; want it untouched",
					servers, tools, probes, rediscoveries)
			}
		})
	}
}

// A person holds a credential only for a server their scopes reach (NT-019).
func TestPutMCPUserCredential_aServerTheCallerDoesNotReach_isRefused(t *testing.T) {
	s := mcpWorld(t)
	putMCP(t, s, as(domain.RoleCurator), "cx-jira", atCX, false)
	put := func(caller context.Context) openapi.PutMCPUserCredentialResponseObject {
		t.Helper()
		resp, err := s.PutMCPUserCredential(caller, openapi.PutMCPUserCredentialRequestObject{
			Name: "cx-jira", Body: &openapi.PutMCPUserCredentialJSONRequestBody{Token: ptr("mine")},
		})
		if err != nil {
			t.Fatalf("PutMCPUserCredential: %v", err)
		}
		return resp
	}
	if _, ok := put(grantedAt(domain.RoleAuthor, finance)).(openapi.PutMCPUserCredential204Response); ok {
		t.Error("an author of finance stored a credential for cx's server")
	}
	if resp := put(grantedAt(domain.RoleAuthor, cx)); func() bool { _, ok := resp.(openapi.PutMCPUserCredential204Response); return !ok }() {
		t.Errorf("an author of cx = %T, want 204", resp)
	}
}

// A sibling's server whose credential will not open answers as any refusal:
// the decision is made before the vault is reached (NT-019).
func TestGetMCPServer_aSiblingsUnreadableSecret_isTheSameRefusal(t *testing.T) {
	pool := testdb.Isolated(t)
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	integrations := admin.NewIntegrations(pool, settings.NewStore(pool, v))
	allowAcmeEgress(t, integrations)
	if _, err := integrations.WriteMCPServer(context.Background(), admin.MCPServerWrite{
		By: "usr_fin", Scope: finance, At: settings.ScopeArea, Stored: finance,
		Server: domain.MCPServer{Name: "fin-jira", Transport: domain.TransportHTTP, URL: "https://mcp.example.com/fin", Enabled: true},
		Given:  domain.MCPCredentialPatch{Token: ptr("fin-token")},
	}); err != nil {
		t.Fatalf("the sibling's server: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`update settings set secret = 'not a sealed document' where kind = 'mcp_server' and name = 'fin-jira'`); err != nil {
		t.Fatalf("spoil the secret: %v", err)
	}
	s := NewServer(ledger.NewMemory(), "test").WithAdministration(nil, nil, integrations)

	resp, err := s.GetMCPServer(grantedAt(domain.RoleAdmin, cx), openapi.GetMCPServerRequestObject{Name: "fin-jira"})
	if err != nil {
		t.Fatalf("GetMCPServer = error %v, want the same 403 as for a name nobody holds", err)
	}
	if _, ok := resp.(openapi.GetMCPServer403ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T, want 403", resp)
	}
}

// A grant on a company reaches its areas' servers as it reaches the company's:
// containment runs both ways, inherited and governed (NT-019).
func TestPutMCPUserCredential_aCompanyGrant_reachesItsAreasServer(t *testing.T) {
	s := mcpWorld(t)
	putMCP(t, s, as(domain.RoleCurator), "cx-jira", atCX, false)
	resp, err := s.PutMCPUserCredential(grantedAt(domain.RoleAuthor, acmeOnly), openapi.PutMCPUserCredentialRequestObject{
		Name: "cx-jira", Body: &openapi.PutMCPUserCredentialJSONRequestBody{Token: ptr("mine")},
	})
	if err != nil {
		t.Fatalf("PutMCPUserCredential: %v", err)
	}
	if _, ok := resp.(openapi.PutMCPUserCredential204Response); !ok {
		t.Errorf("an author of acme = %T, want 204 for a server of acme/cx", resp)
	}
}

/*
A scope a write names is a real company or area, never the installation's
sentinel in disguise (NT-019).

"*" is the installation's company: a row stored at company/* is read back as
the installation's, so the server would be reported at the wrong scope and the
worker could not open its row again. An identifier the platform would store
in another form (upper case, spaces) is refused rather than folded: the row
must be the one the caller named. And a company or area without a scope kind
is refused, not silently written at the installation.
*/
func TestConnectorScope_refusesIdentitiesNoCompanyOrAreaHas(t *testing.T) {
	t.Parallel()
	company, area := openapi.ConnectorScopeKindCompany, openapi.ConnectorScopeKindArea
	for _, one := range []struct {
		name          string
		kind          openapi.ConnectorScopeKind
		company, area *string
	}{
		{"the installation as a company", company, ptr("*"), nil},
		{"the installation as an area's company", area, ptr("*"), ptr("cx")},
		{"a company in upper case", company, ptr("Acme"), nil},
		{"a company with a space", company, ptr("ac me"), nil},
		{"a company with spaces around it", company, ptr(" acme "), nil},
		{"an area in upper case", area, ptr("acme"), ptr("CX")},
		{"an area with a space", area, ptr("acme"), ptr("c x")},
		{"an area with a slash", area, ptr("acme"), ptr("cx/finance")},
	} {
		if scope, err := connectorScope(one.kind, one.company, one.area); err == nil {
			t.Errorf("%s: accepted as %v, want refused", one.name, scope)
		}
	}
	if _, err := connectorScope(area, ptr("acme"), ptr("cx")); err != nil {
		t.Errorf("acme/cx: %v, want accepted", err)
	}
}

func TestPutMCPServer_aScopeWithNoKind_isRefusedRatherThanStoredAtTheInstallation(t *testing.T) {
	s := mcpWorld(t)
	for name, body := range map[string]openapi.PutMCPServerJSONRequestBody{
		"no-kind-company": {Company: ptr("acme")},
		"no-kind-area":    {Company: ptr("acme"), Area: ptr("cx")},
		"star-company":    {ScopeKind: ptr(openapi.ConnectorScopeKindCompany), Company: ptr("*")},
	} {
		body.Transport, body.Url = ptr(openapi.TransportHttp), ptr("https://mcp.example.com/"+name)
		resp, err := s.PutMCPServer(as(domain.RoleCurator), openapi.PutMCPServerRequestObject{Name: name, Body: &body})
		if err != nil {
			t.Fatalf("PutMCPServer %s: %v", name, err)
		}
		if _, ok := resp.(openapi.PutMCPServer400ApplicationProblemPlusJSONResponse); !ok {
			t.Errorf("%s = %T, want 400", name, resp)
		}
	}
}

// The door answers 400 for an area's server outside its scope's egress policy.
func TestPutMCPServer_anAreasServerOutsideItsEgressPolicy_isABadRequest(t *testing.T) {
	s := mcpWorld(t)
	body := openapi.PutMCPServerJSONRequestBody{
		ScopeKind: ptr(atCX.kind), Company: atCX.company, Area: atCX.area,
		Transport: ptr(openapi.TransportHttp), Url: ptr("https://elsewhere.example.org/mcp?tenant=secret"),
	}
	resp, err := s.PutMCPServer(as(domain.RoleCurator), openapi.PutMCPServerRequestObject{Name: "cx-elsewhere", Body: &body})
	if err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	bad, ok := resp.(openapi.PutMCPServer400ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("PutMCPServer = %T, want 400", resp)
	}
	// Its own type, for the console to decide by, and the canonical origin as
	// the whole detail: no path, no query, no internal sentence.
	if valueOr(bad.Type) != "fuseone:egress-not-allowed" || valueOr(bad.Detail) != "https://elsewhere.example.org:443" {
		t.Errorf("problem = %s %q, want fuseone:egress-not-allowed with the origin alone", valueOr(bad.Type), valueOr(bad.Detail))
	}
}
