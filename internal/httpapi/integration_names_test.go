package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
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
A connector instance's name is unique across the installation (NT-019, phase 0).

Journeyed from the handler to the database through the real admin store, so a
layer in the middle that wraps the refusal with %v instead of %w turns the 409
back into a 400 and this test says so.
*/
func TestPutConnectorInstance_aNameAnotherScopeHolds_answers409WithoutNamingTheHolder(t *testing.T) {
	pool := testdb.Isolated(t)
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	server := NewServer(ledger.NewMemory(), "test").
		WithConnectorInstances(admin.NewConnectorInstances(pool, settings.NewStore(pool, v)))

	put := func(area string) openapi.PutConnectorInstanceResponseObject {
		t.Helper()
		token := "vault-token"
		resp, err := server.PutConnectorInstance(as(domain.RoleCurator), openapi.PutConnectorInstanceRequestObject{
			Name: "prod",
			Body: &openapi.PutConnectorInstanceJSONRequestBody{
				Connector: "vault", ScopeKind: openapi.ConnectorScopeKindArea,
				Company: ptr("acme"), Area: ptr(area), Token: &token,
				Vault: &openapi.ConnectorVaultConfig{
					Address: "https://vault.example", Mount: "secret",
					AllowedPathPrefixes: []string{"certs"},
				},
			},
		})
		if err != nil {
			t.Fatalf("PutConnectorInstance in %s: %v", area, err)
		}
		return resp
	}

	if resp := put("cx"); func() bool { _, ok := resp.(openapi.PutConnectorInstance204Response); return !ok }() {
		t.Fatalf("first holder = %T, want 204", resp)
	}
	resp := put("finance")
	taken, ok := resp.(openapi.PutConnectorInstance409ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("second scope = %T, want 409", resp)
	}
	rec := httptest.NewRecorder()
	if err := taken.VisitPutConnectorInstanceResponse(rec); err != nil {
		t.Fatalf("render: %v", err)
	}
	var problem struct {
		Type   string `json:"type"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	if problem.Type != string(CodeIntegrationNameTaken) {
		t.Errorf("type = %q, want %q", problem.Type, CodeIntegrationNameTaken)
	}
	if strings.Contains(rec.Body.String(), "cx") {
		t.Errorf("the answer %s names the scope that holds the name", rec.Body)
	}
}
