package httpapi

import (
	"context"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/vault"
)

/*
An area connects its own connector instances (NT-019, phase 1).

The instance already carries its scope, and the runtime already refuses an
instance outside a run's scope. What changes is who may write one: the
permission at the instance's own scope, or at the platform's administration,
which governs every area.
*/

// grantedAt is a caller holding role at one scope.
func grantedAt(role domain.Role, scope domain.Scope) context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: scope, Role: role}},
	})
}

var (
	cx       = domain.Scope{Company: "acme", Area: "cx"}
	finance  = domain.Scope{Company: "acme", Area: "finance"}
	acmeOnly = domain.Scope{Company: "acme"}
)

// target is where a write lands, as the API names it.
type target struct {
	kind    openapi.ConnectorScopeKind
	company *string
	area    *string
}

var (
	atInstallation = target{kind: openapi.ConnectorScopeKindInstallation}
	atCX           = target{openapi.ConnectorScopeKindArea, ptr("acme"), ptr("cx")}
	atFinance      = target{openapi.ConnectorScopeKindArea, ptr("acme"), ptr("finance")}
	atAcme         = target{openapi.ConnectorScopeKindCompany, ptr("acme"), nil}
)

func putAt(t *testing.T, ctx context.Context, at target) openapi.PutConnectorInstanceResponseObject {
	t.Helper()
	token := "vault-token"
	resp, err := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(&connectorInstanceSpy{}).
		PutConnectorInstance(ctx, openapi.PutConnectorInstanceRequestObject{
			Name: "prod",
			Body: &openapi.PutConnectorInstanceJSONRequestBody{
				Connector: "vault", ScopeKind: at.kind, Company: at.company, Area: at.area,
				Token: &token,
				Vault: &openapi.ConnectorVaultConfig{
					Address: "https://vault.example", Mount: "secret", AllowedPathPrefixes: []string{"certs"},
				},
			},
		})
	if err != nil {
		t.Fatalf("PutConnectorInstance: %v", err)
	}
	return resp
}

func TestPutConnectorInstance_isAuthorizedAtTheInstancesScope(t *testing.T) {
	t.Parallel()
	for _, one := range []struct {
		name    string
		caller  context.Context
		at      target
		written bool
	}{
		{"an area admin in its area", grantedAt(domain.RoleAdmin, cx), atCX, true},
		{"an area admin in a sibling area", grantedAt(domain.RoleAdmin, cx), atFinance, false},
		{"an area admin at its company", grantedAt(domain.RoleAdmin, cx), atAcme, false},
		{"an area admin at the installation", grantedAt(domain.RoleAdmin, cx), atInstallation, false},
		{"a company admin in one of its areas", grantedAt(domain.RoleAdmin, acmeOnly), atFinance, true},
		{"a company admin at its company", grantedAt(domain.RoleAdmin, acmeOnly), atAcme, true},
		{"a company admin at the installation", grantedAt(domain.RoleAdmin, acmeOnly), atInstallation, false},
		// The platform's administration governs every area, and keeps the
		// installation's rows it writes today.
		{"the platform at the installation", as(domain.RoleCurator), atInstallation, true},
		{"the platform in an area", as(domain.RoleCurator), atCX, true},
		{"an author in its area", grantedAt(domain.RoleAuthor, cx), atCX, false},
	} {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()
			resp := putAt(t, one.caller, one.at)
			_, ok := resp.(openapi.PutConnectorInstance204Response)
			_, refused := resp.(openapi.PutConnectorInstance403ApplicationProblemPlusJSONResponse)
			if one.written && !ok {
				t.Errorf("response = %T, want 204", resp)
			}
			if !one.written && !refused {
				t.Errorf("response = %T, want 403", resp)
			}
		})
	}
}

func TestDeleteAndGetConnectorInstance_areAuthorizedAtTheInstancesScope(t *testing.T) {
	t.Parallel()
	server := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(&connectorInstanceSpy{})
	for name, at := range map[string]target{"own area": atCX, "sibling area": atFinance} {
		params := openapi.DeleteConnectorInstanceParams{ScopeKind: at.kind, Company: at.company, Area: at.area}
		deleted, err := server.DeleteConnectorInstance(grantedAt(domain.RoleAdmin, cx),
			openapi.DeleteConnectorInstanceRequestObject{Name: "prod", Params: params})
		if err != nil {
			t.Fatalf("%s: delete: %v", name, err)
		}
		_, refused := deleted.(openapi.DeleteConnectorInstance403ApplicationProblemPlusJSONResponse)
		if refused != (name == "sibling area") {
			t.Errorf("%s: delete = %T", name, deleted)
		}
		got, err := server.GetConnectorInstance(grantedAt(domain.RoleAdmin, cx),
			openapi.GetConnectorInstanceRequestObject{Name: "prod", Params: openapi.GetConnectorInstanceParams{
				ScopeKind: at.kind, Company: at.company, Area: at.area}})
		if err != nil {
			t.Fatalf("%s: get: %v", name, err)
		}
		_, refused = got.(openapi.GetConnectorInstance403ApplicationProblemPlusJSONResponse)
		if refused != (name == "sibling area") {
			t.Errorf("%s: get = %T", name, got)
		}
	}
}

func instanceAt(name string, kind settings.ScopeKind, scope domain.Scope) connectortools.ConfiguredInstance {
	return connectortools.ConfiguredInstance{
		Instance:  connectortools.Instance{Name: name, Connector: "vault", Enabled: true, Scope: scope},
		ScopeKind: kind,
	}
}

func TestListConnectorInstances_showsWhatTheCallerUsesAndGoverns(t *testing.T) {
	t.Parallel()
	spy := &connectorInstanceSpy{items: []connectortools.ConfiguredInstance{
		instanceAt("platform-vault", settings.ScopeInstallation, domain.Scope{}),
		instanceAt("acme-vault", settings.ScopeCompany, acmeOnly),
		instanceAt("cx-vault", settings.ScopeArea, cx),
		instanceAt("finance-vault", settings.ScopeArea, finance),
	}}
	server := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(spy)

	for _, one := range []struct {
		name     string
		caller   context.Context
		listed   []string
		editable []string
	}{
		{"an area admin", grantedAt(domain.RoleAdmin, cx),
			[]string{"acme-vault", "cx-vault", "platform-vault"}, []string{"cx-vault"}},
		{"a company admin", grantedAt(domain.RoleAdmin, acmeOnly),
			[]string{"acme-vault", "cx-vault", "finance-vault", "platform-vault"},
			[]string{"acme-vault", "cx-vault", "finance-vault"}},
		{"the platform", as(domain.RoleCurator),
			[]string{"acme-vault", "cx-vault", "finance-vault", "platform-vault"},
			[]string{"acme-vault", "cx-vault", "finance-vault", "platform-vault"}},
	} {
		resp, err := server.ListConnectorInstances(one.caller, openapi.ListConnectorInstancesRequestObject{})
		if err != nil {
			t.Fatalf("%s: %v", one.name, err)
		}
		list, ok := resp.(openapi.ListConnectorInstances200JSONResponse)
		if !ok {
			t.Fatalf("%s: response = %T, want 200", one.name, resp)
		}
		var listed, editable []string
		for _, item := range list.Items {
			listed = append(listed, item.Name)
			if item.Editable {
				editable = append(editable, item.Name)
			}
		}
		slices.Sort(listed)
		slices.Sort(editable)
		if !slices.Equal(listed, one.listed) || !slices.Equal(editable, one.editable) {
			t.Errorf("%s: listed %v editable %v, want %v and %v", one.name, listed, editable, one.listed, one.editable)
		}
	}
}

// An author holds tool:read and not provider:write: a configuration listing
// carries addresses, so it is not theirs to read.
func TestListConnectorInstances_anAuthor_isRefused(t *testing.T) {
	t.Parallel()
	server := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(&connectorInstanceSpy{})
	resp, err := server.ListConnectorInstances(grantedAt(domain.RoleAuthor, cx), openapi.ListConnectorInstancesRequestObject{})
	if err != nil {
		t.Fatalf("ListConnectorInstances: %v", err)
	}
	if _, ok := resp.(openapi.ListConnectorInstances403ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T, want 403", resp)
	}
}

// The catalogue names connector types, never an instance or a credential.
func TestListConnectorCatalog_isReadWithToolReadInAnyScope(t *testing.T) {
	t.Parallel()
	resp, err := NewServer(ledger.NewMemory(), "test").
		ListConnectorCatalog(grantedAt(domain.RoleAuthor, cx), openapi.ListConnectorCatalogRequestObject{})
	if err != nil {
		t.Fatalf("ListConnectorCatalog: %v", err)
	}
	if _, ok := resp.(openapi.ListConnectorCatalog200JSONResponse); !ok {
		t.Errorf("response = %T, want 200", resp)
	}
}

/*
An area's SQL instance binds only a Vault its own scope can reach (NT-019).

From the area's door, through the real store: the Vault is the sibling area's,
so the binding is refused whatever the area administrator may write. The same
binding to the area's own Vault is accepted, so the refusal is the scope and
not the configuration.
*/
func TestPutConnectorInstance_anAreasBindingToASiblingsVault_isRefused(t *testing.T) {
	pool := testdb.Isolated(t)
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	server := NewServer(ledger.NewMemory(), "test").
		WithConnectorInstances(admin.NewConnectorInstances(pool, settings.NewStore(pool, v)))
	vaultAt := func(caller context.Context, name string, at target) openapi.PutConnectorInstanceResponseObject {
		t.Helper()
		token := "vault-token"
		resp, err := server.PutConnectorInstance(caller, openapi.PutConnectorInstanceRequestObject{
			Name: name, Body: &openapi.PutConnectorInstanceJSONRequestBody{
				Connector: "vault", ScopeKind: at.kind, Company: at.company, Area: at.area, Token: &token,
				Vault: &openapi.ConnectorVaultConfig{Address: "https://vault.example", Mount: "database", AllowedPathPrefixes: []string{"database"}},
			}})
		if err != nil {
			t.Fatalf("put vault %s: %v", name, err)
		}
		return resp
	}
	sqlAt := func(bound string) openapi.PutConnectorInstanceResponseObject {
		t.Helper()
		body := sqlBody()
		body.CredentialSource.VaultInstance = bound
		resp, err := server.PutConnectorInstance(grantedAt(domain.RoleAdmin, cx), openapi.PutConnectorInstanceRequestObject{
			Name: "app-x", Body: &openapi.PutConnectorInstanceJSONRequestBody{
				Connector: "sql", ScopeKind: atCX.kind, Company: atCX.company, Area: atCX.area, Sql: body,
			}})
		if err != nil {
			t.Fatalf("put sql bound to %s: %v", bound, err)
		}
		return resp
	}

	if resp := vaultAt(grantedAt(domain.RoleAdmin, finance), "finance-secrets", atFinance); !isWritten(resp) {
		t.Fatalf("finance's vault = %T", resp)
	}
	if resp := sqlAt("finance-secrets"); isWritten(resp) {
		t.Fatal("an area's SQL instance bound a sibling area's vault")
	}
	if resp := vaultAt(grantedAt(domain.RoleAdmin, cx), "cx-secrets", atCX); !isWritten(resp) {
		t.Fatalf("cx's vault = %T", resp)
	}
	if resp := sqlAt("cx-secrets"); !isWritten(resp) {
		t.Fatalf("binding the area's own vault = %T, want 204", resp)
	}
}

func isWritten(resp openapi.PutConnectorInstanceResponseObject) bool {
	_, ok := resp.(openapi.PutConnectorInstance204Response)
	return ok
}

// Whoever configures nowhere is refused before the body is read: what they may
// not do is not explained to them through its validation errors.
func TestPutConnectorInstance_aCallerWhoConfiguresNowhere_isRefusedBeforeTheBodyIsJudged(t *testing.T) {
	t.Parallel()
	resp, err := NewServer(ledger.NewMemory(), "test").WithConnectorInstances(&connectorInstanceSpy{}).
		PutConnectorInstance(grantedAt(domain.RoleAuthor, cx), openapi.PutConnectorInstanceRequestObject{Name: "prod"})
	if err != nil {
		t.Fatalf("PutConnectorInstance: %v", err)
	}
	if _, ok := resp.(openapi.PutConnectorInstance403ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T, want 403 before the missing body is judged", resp)
	}
}
