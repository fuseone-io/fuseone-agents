package httpapi

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// fakeAdmin records what the handlers asked for, so a test can assert on the
// call rather than on a database.
type fakeAdmin struct {
	servers   []domain.MCPServer
	providers []domain.ModelProvider

	putServer   domain.MCPServer
	putProvider domain.ModelProvider
	putKey      string
	putBy       domain.UserID
	deleted     string
	err         error
}

func (f *fakeAdmin) MCPServers(context.Context) ([]domain.MCPServer, error) {
	return f.servers, f.err
}
func (f *fakeAdmin) Providers(context.Context) ([]domain.ModelProvider, error) {
	return f.providers, f.err
}
func (f *fakeAdmin) PutMCPServer(_ context.Context, by domain.UserID, _ domain.Scope, s domain.MCPServer) error {
	f.putServer, f.putBy = s, by
	return f.err
}
func (f *fakeAdmin) DeleteMCPServer(_ context.Context, by domain.UserID, _ domain.Scope, name string) error {
	f.deleted, f.putBy = name, by
	return f.err
}
func (f *fakeAdmin) PutProvider(_ context.Context, by domain.UserID, _ domain.Scope, p domain.ModelProvider, key string) error {
	f.putProvider, f.putKey, f.putBy = p, key, by
	return f.err
}
func (f *fakeAdmin) DeleteProvider(_ context.Context, by domain.UserID, _ domain.Scope, name string) error {
	f.deleted, f.putBy = name, by
	return f.err
}

func serverWith(t *testing.T, admin *fakeAdmin) *Server {
	t.Helper()
	return NewServer(ledger.NewMemory(), "test").WithAdministration(nil, nil, admin)
}

// as returns a context carrying a caller with the given role in the scope the
// administration area is authorised in.
func as(role domain.Role) context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: adminScope, Role: role}},
	})
}

func TestPutMCPServer_withoutThePermission_isRefused(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	// An auditor reads everything and changes nothing. Hiding the screen from
	// them would be a courtesy; this is the control.
	resp, err := serverWith(t, admin).PutMCPServer(as(domain.RoleAuditor), openapi.PutMCPServerRequestObject{
		Name: "crm", Body: &openapi.PutMCPServerJSONRequestBody{Command: "/usr/bin/crm-mcp"},
	})
	if err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if _, refused := resp.(openapi.PutMCPServer403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
	if admin.putServer.Command != "" {
		t.Error("a refused request still reached the administration store")
	}
}

func TestPutMCPServer_withoutAnyCredential_isRefused(t *testing.T) {
	t.Parallel()

	// No principal in the context at all: a handler reached without
	// authentication must fail closed rather than act as a zero principal.
	resp, err := serverWith(t, &fakeAdmin{}).PutMCPServer(context.Background(),
		openapi.PutMCPServerRequestObject{Name: "crm", Body: &openapi.PutMCPServerJSONRequestBody{Command: "x"}})
	if err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if _, refused := resp.(openapi.PutMCPServer403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want a refusal", resp)
	}
}

func TestPutMCPServer_attributesTheChangeToTheCaller(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	resp, err := serverWith(t, admin).PutMCPServer(as(domain.RoleCurator), openapi.PutMCPServerRequestObject{
		Name: "crm",
		Body: &openapi.PutMCPServerJSONRequestBody{Command: "bin/devstack", Args: &[]string{"mcp"}},
	})
	if err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if _, ok := resp.(openapi.PutMCPServer204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	// Every administrative change is somebody's, never the platform's.
	if admin.putBy != "usr_ana" {
		t.Errorf("recorded by %q, want the caller", admin.putBy)
	}
	if admin.putServer.Command != "bin/devstack" || len(admin.putServer.Args) != 1 {
		t.Errorf("stored = %+v, want the command and its arguments", admin.putServer)
	}
}

func TestPutModelProvider_withoutAKey_passesNoneSoTheStoredOneSurvives(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	_, err := serverWith(t, admin).PutModelProvider(as(domain.RoleCurator), openapi.PutModelProviderRequestObject{
		Name: "openai",
		Body: &openapi.PutModelProviderJSONRequestBody{Kind: "openai_compatible", BaseUrl: "https://a.example"},
	})
	if err != nil {
		t.Fatalf("PutModelProvider: %v", err)
	}
	// Changing a base URL must not demand re-entering the credential.
	if admin.putKey != "" {
		t.Errorf("apiKey = %q, want empty so the stored credential is kept", admin.putKey)
	}
}

func TestListIntegrations_neverReturnsACredential(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{providers: []domain.ModelProvider{
		{Name: "openai", Kind: "openai_compatible", BaseURL: "https://a.example", HasKey: true, Enabled: true},
	}}

	resp, err := serverWith(t, admin).ListIntegrations(as(domain.RoleCurator), openapi.ListIntegrationsRequestObject{})
	if err != nil {
		t.Fatalf("ListIntegrations: %v", err)
	}
	body, ok := resp.(openapi.ListIntegrations200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", resp)
	}
	if len(body.Providers) != 1 || !body.Providers[0].HasKey {
		t.Fatalf("providers = %+v, want one reporting a stored key", body.Providers)
	}
	// The shape itself is the guarantee: there is nowhere for a secret to go.
	if body.Providers[0].BaseUrl != "https://a.example" {
		t.Errorf("baseUrl = %q, want it rendered", body.Providers[0].BaseUrl)
	}
}

func TestDeleteMCPServer_removingATooSourceIsAllowedForTheCurator(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	resp, err := serverWith(t, admin).DeleteMCPServer(as(domain.RoleCurator),
		openapi.DeleteMCPServerRequestObject{Name: "crm"})
	if err != nil {
		t.Fatalf("DeleteMCPServer: %v", err)
	}
	if _, ok := resp.(openapi.DeleteMCPServer204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if admin.deleted != "crm" {
		t.Errorf("deleted = %q, want crm", admin.deleted)
	}
}

func TestPutMCPServer_storeRefuses_readsBackAsABadRequestNotAServerError(t *testing.T) {
	t.Parallel()

	// A command that cannot be run is the operator's mistake, and telling them
	// so is more useful than a 500 that reads as the platform being broken.
	admin := &fakeAdmin{err: errors.New("an MCP server needs a command to run")}
	resp, err := serverWith(t, admin).PutMCPServer(as(domain.RoleCurator), openapi.PutMCPServerRequestObject{
		Name: "crm", Body: &openapi.PutMCPServerJSONRequestBody{Command: " "},
	})
	if err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if _, bad := resp.(openapi.PutMCPServer400ApplicationProblemPlusJSONResponse); !bad {
		t.Fatalf("response = %T, want 400", resp)
	}
}
