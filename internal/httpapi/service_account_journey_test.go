package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

// From a real token, through the authenticator and the routes, to the handlers. A
// test that sets the principal directly is blind to whether a token ever produces it.
type machine struct {
	handler http.Handler
	secret  string
}

func (m machine) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.secret)
	rec := httptest.NewRecorder()
	m.handler.ServeHTTP(rec, req)
	return rec
}

func routed(dir *auth.Postgres, server *Server) http.Handler {
	api := openapi.HandlerWithOptions(
		openapi.NewStrictHandlerWithOptions(server, nil, openapi.StrictHTTPServerOptions{}),
		openapi.StdHTTPServerOptions{BaseURL: "/api/v1", BaseRouter: http.NewServeMux()},
	)
	return auth.NewAuthenticator(dir, false, nil).Middleware(api)
}

func issuedFor(
	t *testing.T, dir *auth.Postgres, name string, role domain.Role, scope domain.Scope,
) (domain.ServiceAccount, auth.IssuedToken) {
	t.Helper()
	ctx := context.Background()
	pool := testdb.Pool(t)
	for _, table := range []string{"api_tokens", "role_grants", "principals"} {
		if _, err := pool.Exec(ctx, `delete from `+table+` where principal_id = $1`, "svc_"+name); err != nil {
			t.Fatalf("clean %s: %v", table, err)
		}
	}
	account, err := dir.CreateServiceAccount(ctx, name, "journey", "usr_admin")
	if err != nil {
		t.Fatalf("CreateServiceAccount: %v", err)
	}
	if err := dir.SetGrants(ctx, account.ID, []domain.Grant{{Scope: scope, Role: role}}, "usr_admin"); err != nil {
		t.Fatalf("SetGrants: %v", err)
	}
	issued, err := dir.IssueServiceToken(ctx, account.ID, "main", "usr_admin", time.Now().Add(time.Hour), time.Now())
	if err != nil {
		t.Fatalf("IssueServiceToken: %v", err)
	}
	return account, issued
}

func TestServiceToken_publishesAnAgent_andRevokingStopsItOnTheNextRequest(t *testing.T) {
	dir := auth.NewPostgres(testdb.Pool(t))
	account, issued := issuedFor(t, dir, "journey-publisher", domain.RoleAuthor, domain.Scope{Company: "acme", Area: "cx"})
	pub := newPublisher()
	token := machine{routed(dir, publishServer(t, pub)), issued.Secret}

	published := token.do(t, http.MethodPut, "/agents/atendimento/versions", definition(nil))
	if published.Code != http.StatusOK && published.Code != http.StatusCreated {
		t.Fatalf("publish = %d: %s", published.Code, published.Body)
	}

	elsewhere := token.do(t, http.MethodPut, "/agents/atendimento/versions", definition(func(d *openapi.AgentDefinition) {
		d.Area = "finance"
	}))
	if elsewhere.Code != http.StatusForbidden {
		t.Fatalf("publish outside its scope = %d, want 403", elsewhere.Code)
	}

	if err := dir.RevokeServiceToken(context.Background(), account.ID, issued.ID, "usr_admin", time.Now()); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if after := token.do(t, http.MethodPut, "/agents/atendimento/versions", definition(nil)); after.Code != http.StatusUnauthorized {
		t.Fatalf("publish after revoking = %d, want 401", after.Code)
	}
}

func TestServiceToken_configuresAConnector(t *testing.T) {
	dir := auth.NewPostgres(testdb.Pool(t))
	_, issued := issuedFor(t, dir, "journey-curator", domain.RoleCurator, adminScope)
	spy := &connectorInstanceSpy{}
	token := machine{routed(dir, NewServer(ledger.NewMemory(), "test").WithConnectorInstances(spy)), issued.Secret}

	put := token.do(t, http.MethodPut, "/admin/integrations/connectors/instances/gateway",
		openapi.PutConnectorInstanceJSONRequestBody{
			Connector: "evidence", ScopeKind: openapi.ConnectorScopeKindArea,
			Company: ptr("acme"), Area: ptr("cyber"), Token: ptr("CANARY-token"),
			Evidence: evidenceAPIConfig(t),
		})

	if put.Code != http.StatusNoContent {
		t.Fatalf("put = %d: %s", put.Code, put.Body)
	}
	if spy.put.Evidence.BaseURL == "" {
		t.Fatal("the declaration did not arrive")
	}
}

// The adversary's move: a token that was given the role, calling the real route. If a
// grant were enough, one leaked secret would be an approval.
func TestServiceToken_withTheApproverRole_cannotApprove(t *testing.T) {
	dir := auth.NewPostgres(testdb.Pool(t))
	_, issued := issuedFor(t, dir, "journey-approver", domain.RoleAdmin, domain.Scope{Company: domain.Installation})
	asked := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	store := awaitingApproval(t, asked)
	token := machine{routed(dir, NewServer(store, "test")), issued.Secret}

	decided := token.do(t, http.MethodPost, "/runs/run-1/approvals",
		openapi.DecideApprovalJSONRequestBody{Approved: true, AtSeq: 2})

	if decided.Code != http.StatusForbidden {
		t.Fatalf("approve = %d: %s", decided.Code, decided.Body)
	}
	steps, err := store.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if last := steps[len(steps)-1]; last.Kind == domain.StepApprovalDecided {
		t.Fatal("the approval was recorded")
	}

	// Anchored: the same route with a person's grants does decide, so the 403 above
	// came from the kind of principal and not from a broken fixture.
	response, err := NewServer(store, "test").WithClock(fixedAt{t: asked.Add(time.Minute)}).DecideApproval(
		inArea("cx", domain.RoleApprover), openapi.DecideApprovalRequestObject{
			RunId: "run-1", Body: &openapi.DecideApprovalJSONRequestBody{Approved: true, AtSeq: 2},
		})
	if err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}
	if _, refused := response.(openapi.DecideApproval403ApplicationProblemPlusJSONResponse); refused {
		t.Fatal("a person was refused too, so this test proves nothing")
	}
}

// A token that could issue tokens or change grants could give itself anything.
func TestServiceToken_cannotManageIdentity_evenAsAdmin(t *testing.T) {
	dir := auth.NewPostgres(testdb.Pool(t))
	account, issued := issuedFor(t, dir, "journey-admin", domain.RoleAdmin, domain.Scope{Company: domain.Installation})
	server := NewServer(ledger.NewMemory(), "test").WithServiceAccounts(dir).WithPeople(dir)
	token := machine{routed(dir, server), issued.Secret}

	moves := []struct {
		name, method, path string
		body               any
	}{
		{"list accounts", http.MethodGet, "/admin/service-accounts", nil},
		{"create an account", http.MethodPost, "/admin/service-accounts", map[string]string{"name": "another"}},
		{"issue itself a token", http.MethodPost, "/admin/service-accounts/" + account.ID + "/tokens",
			map[string]any{"name": "x", "expiresAt": time.Now().Add(time.Hour)}},
		{"re-enable an account", http.MethodPut, "/admin/service-accounts/" + account.ID + "/disabled",
			map[string]bool{"disabled": false}},
		{"revoke a token", http.MethodDelete, "/admin/service-accounts/" + account.ID + "/tokens/" + issued.ID, nil},
		{"grant itself more", http.MethodPut, "/admin/people/" + account.ID + "/grants",
			map[string]any{"grants": []map[string]string{{"company": "*", "area": "", "role": "admin"}}}},
	}
	for _, move := range moves {
		if got := token.do(t, move.method, move.path, move.body); got.Code != http.StatusForbidden {
			t.Errorf("%s = %d, want 403: %s", move.name, got.Code, got.Body)
		}
	}
}

// The secret exists in one response. Everywhere else it is a hash nobody can use.
func TestServiceTokenRoutes_returnTheSecretOnlyWhenIssuing(t *testing.T) {
	dir := auth.NewPostgres(testdb.Pool(t))
	server := NewServer(ledger.NewMemory(), "test").WithServiceAccounts(dir)
	refused, err := server.CreateServiceAccount(inArea("cx", domain.RoleAuthor), openapi.CreateServiceAccountRequestObject{
		Body: &openapi.ServiceAccountInput{Name: "not-allowed"},
	})
	if err != nil {
		t.Fatalf("CreateServiceAccount: %v", err)
	}
	if _, ok := refused.(openapi.CreateServiceAccount403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("an author got %T, want 403", refused)
	}

	admin := auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_admin", Kind: domain.PrincipalUser, Grants: []domain.Grant{{
			Scope: domain.Scope{Company: domain.Installation}, Role: domain.RoleAdmin,
		}},
	})
	pool := testdb.Pool(t)
	for _, table := range []string{"api_tokens", "principals"} {
		if _, err := pool.Exec(context.Background(),
			`delete from `+table+` where principal_id = 'svc_listed-route'`); err != nil {
			t.Fatalf("clean: %v", err)
		}
	}
	created, err := server.CreateServiceAccount(admin, openapi.CreateServiceAccountRequestObject{
		Body: &openapi.ServiceAccountInput{Name: "listed-route"},
	})
	if err != nil {
		t.Fatalf("CreateServiceAccount: %v", err)
	}
	account, ok := created.(openapi.CreateServiceAccount201JSONResponse)
	if !ok {
		t.Fatalf("response = %T", created)
	}
	issuedResp, err := server.IssueServiceToken(admin, openapi.IssueServiceTokenRequestObject{
		AccountId: account.Id,
		Body:      &openapi.ServiceTokenInput{Name: "main", ExpiresAt: time.Now().Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("IssueServiceToken: %v", err)
	}
	issued, ok := issuedResp.(openapi.IssueServiceToken201JSONResponse)
	if !ok || issued.Secret == "" {
		t.Fatalf("response = %+v", issuedResp)
	}

	listed, err := server.ListServiceTokens(admin, openapi.ListServiceTokensRequestObject{AccountId: account.Id})
	if err != nil {
		t.Fatalf("ListServiceTokens: %v", err)
	}
	raw, err := json.Marshal(listed)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.Contains(string(raw), issued.Secret) || !strings.Contains(string(raw), issued.Id) {
		t.Fatalf("listing = %s", raw)
	}
}
