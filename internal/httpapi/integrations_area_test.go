package httpapi

import (
	"reflect"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
An area's administrator connects the area's own servers (NT-019), and the
catalogue is where connecting one starts. A recipe names a server other people
publish, nothing of this installation, so whoever may connect a server
somewhere reads it: the act and the screen that starts it share a permission.
*/
func TestListRecipes_anAreaThatConnectsServers_readsTheCatalogue(t *testing.T) {
	t.Parallel()

	for _, role := range []domain.Role{domain.RoleAdmin, domain.RoleCurator} {
		resp, err := NewServer(ledger.NewMemory(), "test").
			ListRecipes(inArea("cx", role), openapi.ListRecipesRequestObject{})
		if err != nil {
			t.Fatalf("ListRecipes(%s): %v", role, err)
		}
		if _, ok := resp.(openapi.ListRecipes200JSONResponse); !ok {
			t.Errorf("%s of an area: response = %T, want the catalogue", role, resp)
		}
	}
}

// Reading tools in an area is not connecting servers there.
func TestListRecipes_anAreaThatCannotConnectServers_isRefused(t *testing.T) {
	t.Parallel()

	for _, role := range []domain.Role{domain.RoleAuthor, domain.RoleAuditor} {
		resp, err := NewServer(ledger.NewMemory(), "test").
			ListRecipes(inArea("cx", role), openapi.ListRecipesRequestObject{})
		if err != nil {
			t.Fatalf("ListRecipes(%s): %v", role, err)
		}
		if _, refused := resp.(openapi.ListRecipes403ApplicationProblemPlusJSONResponse); !refused {
			t.Errorf("%s of an area: response = %T, want 403", role, resp)
		}
	}
}

/*
An area's author reads tools in the area and cannot list the servers'
configuration, which carries addresses. Their personal credentials need the
servers they may hold one for, so the listing names those, asked by the
scopes where they read tools, with no address (NT-019).
*/
func TestListMCPUserCredentials_namesTheServersTheCallerMayHoldACredentialFor(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{credentialServers: []domain.MCPCredentialServer{
		{Name: "crm", Scope: domain.Scope{Company: "acme", Area: "cx"}, Shared: true},
		{Name: "docs"},
	}}
	resp, err := serverWith(t, admin).ListMCPUserCredentials(
		inArea("cx", domain.RoleAuthor), openapi.ListMCPUserCredentialsRequestObject{})
	if err != nil {
		t.Fatalf("ListMCPUserCredentials: %v", err)
	}
	body, ok := resp.(openapi.ListMCPUserCredentials200JSONResponse)
	if !ok || body.Servers == nil {
		t.Fatalf("response = %#v, want servers", resp)
	}
	want := []openapi.MCPCredentialServer{
		{Name: "crm", ScopeKind: openapi.MCPCredentialServerScopeKindArea, Company: ptr("acme"), Area: ptr("cx"), Shared: true},
		{Name: "docs", ScopeKind: openapi.MCPCredentialServerScopeKindInstallation},
	}
	if !reflect.DeepEqual(*body.Servers, want) {
		t.Errorf("servers = %+v, want %+v", *body.Servers, want)
	}
	if len(admin.credentialScopes) != 1 || admin.credentialScopes[0] != (domain.Scope{Company: "acme", Area: "cx"}) {
		t.Errorf("asked for %v, want the scopes where the caller reads tools", admin.credentialScopes)
	}
}
