package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/known"
	"github.com/fuseone/agents/internal/ledger"
)

// fakeAdmin records what the handlers asked for, so a test can assert on the
// call rather than on a database.
type fakeAdmin struct {
	servers   []domain.MCPServer
	personal  []domain.MCPPersonalCredential
	providers []domain.ModelProvider

	putServer   domain.MCPServer
	putPersonal string
	putToken    string
	putHeaders  map[string]string
	putOAuth    *domain.MCPOAuthGrant
	putEnv      map[string]string
	token       string
	putProvider domain.ModelProvider
	putKey      string
	putBy       domain.UserID
	deleted     string
	probed      string
	err         error
}

func (f *fakeAdmin) MCPServers(context.Context) ([]domain.MCPServer, error) {
	return f.servers, f.err
}
func (f *fakeAdmin) Providers(context.Context) ([]domain.ModelProvider, error) {
	return f.providers, f.err
}
func (f *fakeAdmin) MCPPersonalCredentials(context.Context, domain.UserID) ([]domain.MCPPersonalCredential, error) {
	return f.personal, f.err
}
func (f *fakeAdmin) PutMCPPersonalCredential(
	_ context.Context, by domain.UserID, _ domain.Scope, name string,
	creds domain.MCPCredentialPatch,
) error {
	f.putPersonal, f.putBy, f.putHeaders = name, by, creds.Headers
	if creds.Token != nil {
		f.putToken = *creds.Token
	}
	if creds.OAuth != nil {
		copy := *creds.OAuth
		f.putOAuth = &copy
	}
	return f.err
}
func (f *fakeAdmin) DeleteMCPPersonalCredential(_ context.Context, by domain.UserID, _ domain.Scope, name string) error {
	f.deleted, f.putBy = name, by
	return f.err
}
func (f *fakeAdmin) PutMCPServer(
	_ context.Context, by domain.UserID, _ domain.Scope, s domain.MCPServer,
	creds domain.MCPCredentialPatch,
) error {
	f.putServer, f.putBy, f.putEnv, f.putHeaders = s, by, creds.Env, creds.Headers
	if creds.Token != nil {
		f.putToken = *creds.Token
	}
	if creds.OAuth != nil {
		copy := *creds.OAuth
		f.putOAuth = &copy
	}
	return f.err
}

func (f *fakeAdmin) MCPCredentials(context.Context, string) (domain.MCPCredentials, error) {
	return domain.MCPCredentials{Token: f.token}, nil
}
func (f *fakeAdmin) RequestMCPProbe(_ context.Context, by domain.UserID, _ domain.Scope, name string) error {
	f.probed, f.putBy = name, by
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

type eventAdmin struct {
	askedCursor string
	next        string
	events      []domain.AdminEvent
}

func (e *eventAdmin) Classify(context.Context, domain.Scope, domain.ToolClassification) error {
	return nil
}

func (e *eventAdmin) List(context.Context, domain.Scope) ([]domain.ToolClassification, error) {
	return nil, nil
}

func (e *eventAdmin) Events(_ context.Context, _ string, cursor string, _ int) ([]domain.AdminEvent, string, error) {
	e.askedCursor = cursor
	return e.events, e.next, nil
}

// inArea returns a caller holding a role in one area of acme.
func inArea(area string, role domain.Role) context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: domain.AreaID(area)}, Role: role}},
	})
}

// as returns a context carrying a caller with the given role in the scope the
// administration area is authorised in.
func as(role domain.Role) context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: adminScope, Role: role}},
	})
}

// asInstallation returns a caller holding a role at the installation scope.
func asInstallation(role domain.Role) context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: identityScope, Role: role}},
	})
}

func TestPutMCPServer_withoutThePermission_isRefused(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	// An auditor reads everything and changes nothing. Hiding the screen from
	// them would be a courtesy; this is the control.
	resp, err := serverWith(t, admin).PutMCPServer(as(domain.RoleAuditor), openapi.PutMCPServerRequestObject{
		Name: "crm", Body: &openapi.PutMCPServerJSONRequestBody{Command: ptr("/usr/bin/crm-mcp")},
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

func TestListAdminEvents_cursorReachesTheStoreAndTheAnswerCarriesTheNext(t *testing.T) {
	t.Parallel()

	admin := &eventAdmin{
		next: "next-page",
		events: []domain.AdminEvent{{
			At:        time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC),
			Principal: "usr_ana",
			Action:    "tool.classified",
			Target:    "crm.lookup",
		}},
	}
	resp, err := NewServer(ledger.NewMemory(), "test").WithAdministration(admin, nil, nil).
		ListAdminEvents(as(domain.RoleAuditor), openapi.ListAdminEventsRequestObject{
			Params: openapi.ListAdminEventsParams{Cursor: ptr("from-page-one")},
		})
	if err != nil {
		t.Fatalf("ListAdminEvents: %v", err)
	}
	if admin.askedCursor != "from-page-one" {
		t.Errorf("store cursor = %q, want the cursor from the request", admin.askedCursor)
	}
	page, ok := resp.(openapi.ListAdminEvents200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", resp)
	}
	if page.NextCursor == nil || *page.NextCursor != "next-page" {
		t.Errorf("nextCursor = %v, want the one the store returned", page.NextCursor)
	}
}

func TestPutMCPServer_withoutAnyCredential_isRefused(t *testing.T) {
	t.Parallel()

	// No principal in the context at all: a handler reached without
	// authentication must fail closed rather than act as a zero principal.
	resp, err := serverWith(t, &fakeAdmin{}).PutMCPServer(context.Background(),
		openapi.PutMCPServerRequestObject{Name: "crm", Body: &openapi.PutMCPServerJSONRequestBody{Command: ptr("x")}})
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
		Body: &openapi.PutMCPServerJSONRequestBody{Command: ptr("bin/devstack"), Args: &[]string{"mcp"}},
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

func TestPutMCPServer_passesOAuthGrantToTheStore(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	scopes := []string{"sheets.readonly"}
	expires := int64(1893456000)
	resp, err := serverWith(t, admin).PutMCPServer(as(domain.RoleCurator), openapi.PutMCPServerRequestObject{
		Name: "google-workspace",
		Body: &openapi.PutMCPServerJSONRequestBody{
			Transport: ptr(openapi.Http),
			Url:       ptr("https://mcp.example.com/google"),
			Oauth: &openapi.MCPOAuthGrant{
				AccessToken:   ptr("access"),
				RefreshToken:  ptr("refresh"),
				TokenURL:      ptr("https://issuer.example/token"),
				ClientID:      ptr("client"),
				ClientSecret:  ptr("secret"),
				TokenType:     ptr("Bearer"),
				ExpiresAtUnix: ptr(expires),
				Scopes:        &scopes,
			},
		},
	})
	if err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if _, ok := resp.(openapi.PutMCPServer204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if admin.putOAuth == nil ||
		admin.putOAuth.AccessToken != "access" ||
		admin.putOAuth.RefreshToken != "refresh" ||
		admin.putOAuth.ClientSecret != "secret" ||
		admin.putOAuth.ExpiresAtUnix != expires ||
		admin.putOAuth.Scopes[0] != "sheets.readonly" {
		t.Fatalf("oauth = %+v, want the request grant passed to admin", admin.putOAuth)
	}
}

func TestPutMCPServer_passesHeaderCredentialsToTheStore(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	headers := map[string]string{"Api-Key": "nr_secret"}
	resp, err := serverWith(t, admin).PutMCPServer(as(domain.RoleCurator), openapi.PutMCPServerRequestObject{
		Name: "newrelic",
		Body: &openapi.PutMCPServerJSONRequestBody{
			Transport: ptr(openapi.Http),
			Url:       ptr("https://mcp.newrelic.com/mcp/"),
			Headers:   &headers,
		},
	})
	if err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if _, ok := resp.(openapi.PutMCPServer204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if admin.putToken != "" {
		t.Fatalf("token = %q, want no bearer token", admin.putToken)
	}
	if admin.putHeaders["Api-Key"] != "nr_secret" {
		t.Fatalf("headers = %+v, want the request headers passed to admin", admin.putHeaders)
	}
}

func TestProbeMCPServer_recordsWhoAskedForTheWorkerCheck(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	resp, err := serverWith(t, admin).ProbeMCPServer(as(domain.RoleCurator),
		openapi.ProbeMCPServerRequestObject{Name: "stripe"})
	if err != nil {
		t.Fatalf("ProbeMCPServer: %v", err)
	}
	if _, ok := resp.(openapi.ProbeMCPServer202Response); !ok {
		t.Fatalf("response = %T, want 202", resp)
	}
	if admin.probed != "stripe" || admin.putBy != "usr_ana" {
		t.Errorf("probe = %q by %q, want stripe by the caller", admin.probed, admin.putBy)
	}
}

func TestProbeMCPServer_withoutThePermission_isRefused(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	resp, err := serverWith(t, admin).ProbeMCPServer(as(domain.RoleAuditor),
		openapi.ProbeMCPServerRequestObject{Name: "stripe"})
	if err != nil {
		t.Fatalf("ProbeMCPServer: %v", err)
	}
	if _, refused := resp.(openapi.ProbeMCPServer403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want a refusal", resp)
	}
	if admin.probed != "" {
		t.Fatalf("probe was recorded despite the refusal: %q", admin.probed)
	}
}

func TestPutMCPUserCredential_isAllowedForAnAuthorAndStoresOnlyTheirCredential(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	resp, err := serverWith(t, admin).PutMCPUserCredential(
		inArea("devops", domain.RoleAuthor),
		openapi.PutMCPUserCredentialRequestObject{
			Name: "github",
			Body: &openapi.PutMCPUserCredentialJSONRequestBody{
				Token: ptr("ghp_secret"),
			},
		})
	if err != nil {
		t.Fatalf("PutMCPUserCredential: %v", err)
	}
	if _, ok := resp.(openapi.PutMCPUserCredential204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if admin.putPersonal != "github" || admin.putBy != "usr_ana" || admin.putToken != "ghp_secret" {
		t.Fatalf("stored = %q by %q token %q, want Ana's github credential",
			admin.putPersonal, admin.putBy, admin.putToken)
	}
}

func TestListMCPUserCredentials_returnsPresenceOnly(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	admin := &fakeAdmin{personal: []domain.MCPPersonalCredential{{
		Server: "github", Principal: "usr_ana", HasSecret: true,
		HasHeaders: true, UpdatedBy: "usr_ana", UpdatedAt: at,
	}}}
	resp, err := serverWith(t, admin).ListMCPUserCredentials(
		inArea("devops", domain.RoleAuthor),
		openapi.ListMCPUserCredentialsRequestObject{})
	if err != nil {
		t.Fatalf("ListMCPUserCredentials: %v", err)
	}
	body, ok := resp.(openapi.ListMCPUserCredentials200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", resp)
	}
	if len(body.Items) != 1 ||
		body.Items[0].Server != "github" ||
		!body.Items[0].HasCredential ||
		!body.Items[0].HasHeaders ||
		body.Items[0].HasOAuth {
		t.Fatalf("items = %+v, want only credential presence", body.Items)
	}
}

func TestPutModelProvider_withoutAKey_passesNoneSoTheStoredOneSurvives(t *testing.T) {
	t.Parallel()

	admin := &fakeAdmin{}
	_, err := serverWith(t, admin).PutModelProvider(as(domain.RoleCurator), openapi.PutModelProviderRequestObject{
		Name: "openai",
		Body: &openapi.PutModelProviderJSONRequestBody{Kind: "openai_compatible", BaseUrl: ptr("https://a.example")},
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
		Name: "crm", Body: &openapi.PutMCPServerJSONRequestBody{Command: ptr(" ")},
	})
	if err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if _, bad := resp.(openapi.PutMCPServer400ApplicationProblemPlusJSONResponse); !bad {
		t.Fatalf("response = %T, want 400", resp)
	}
}

// fakeAgents stands in for the registry of published versions.
type fakeAgents struct {
	published []domain.AgentSummary
	allAsked  bool
}

func (f *fakeAgents) List(_ context.Context, _ domain.Scope, all bool) ([]domain.AgentSummary, error) {
	f.allAsked = all
	return f.published, nil
}

func (f *fakeAgents) Versions(_ context.Context, agent domain.AgentID) ([]domain.AgentSummary, error) {
	var out []domain.AgentSummary
	for _, a := range f.published {
		if a.ID == agent {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeAgents) Instructions(context.Context, domain.AgentID, domain.VersionID) (string, string, error) {
	return "", "", nil
}

func TestListAgents_askingForAnotherAreaIsRefused(t *testing.T) {
	t.Parallel()

	// An author granted in cx must not read marketing by naming it. Hiding it
	// from the navigation is a courtesy; this is the control (PRD NF-06).
	server := NewServer(ledger.NewMemory(), "test").WithAgents(&fakeAgents{})
	resp, err := server.ListAgents(inArea("cx", domain.RoleAuthor), openapi.ListAgentsRequestObject{
		Params: openapi.ListAgentsParams{Company: ptr("acme"), Area: ptr("marketing")},
	})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if _, refused := resp.(openapi.ListAgents403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
}

func TestListAgents_ownAreaIsNotRefusedForLackingAnAdministrativeGrant(t *testing.T) {
	t.Parallel()

	// Checking every read against the installation's administrative scope
	// would refuse an author their own area, which is the scope they actually
	// hold.
	agents := &fakeAgents{published: []domain.AgentSummary{{
		ID: "triage", Scope: domain.Scope{Company: "acme", Area: "cx"},
	}}}
	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(agents).
		ListAgents(inArea("cx", domain.RoleAuthor), openapi.ListAgentsRequestObject{
			Params: openapi.ListAgentsParams{Company: ptr("acme"), Area: ptr("cx")},
		})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	body, ok := resp.(openapi.ListAgents200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", resp)
	}
	if len(body.Items) != 1 {
		t.Errorf("items = %d, want the agent in the caller's own area", len(body.Items))
	}
}

func TestListAgents_unscopedListShowsOnlyWhatTheCallerMaySee(t *testing.T) {
	t.Parallel()

	// An unscoped question is answered with the caller's own scopes rather
	// than refused with a permission error naming a scope they never mentioned.
	agents := &fakeAgents{published: []domain.AgentSummary{
		{ID: "triage", Scope: domain.Scope{Company: "acme", Area: "cx"}},
		{ID: "leads", Scope: domain.Scope{Company: "acme", Area: "marketing"}},
	}}
	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(agents).
		ListAgents(inArea("cx", domain.RoleAuthor), openapi.ListAgentsRequestObject{})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	body := resp.(openapi.ListAgents200JSONResponse)
	if len(body.Items) != 1 || body.Items[0].AgentId != "triage" {
		t.Errorf("items = %v, want only the caller's area", body.Items)
	}
}

func TestListAgents_rendersTheCapabilityPackAndTheCeilings(t *testing.T) {
	t.Parallel()

	agents := &fakeAgents{published: []domain.AgentSummary{{
		ID: "triage", VersionID: "v1", Name: "Ticket triage",
		Scope:  adminScope,
		Tools:  []domain.ToolID{"crm.lookup"},
		Budget: domain.Budget{Micros: 500_000, Steps: 60},
		Latest: true,
	}}}

	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(agents).
		ListAgents(as(domain.RoleAuthor), openapi.ListAgentsRequestObject{})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	body, ok := resp.(openapi.ListAgents200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 200", resp)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(body.Items))
	}

	// The pack is the whole security story: what is not listed cannot be
	// invoked, so a screen that omits it hides the answer to "what can this
	// agent do".
	got := body.Items[0]
	if len(got.Tools) != 1 || got.Tools[0] != "crm.lookup" {
		t.Errorf("tools = %v, want the capability pack", got.Tools)
	}
	if got.Budget.Steps == nil || *got.Budget.Steps != 60 {
		t.Errorf("budget.steps = %v, want the ceiling as published", got.Budget.Steps)
	}
}

func TestListAgents_historyIsAskedForExplicitly(t *testing.T) {
	t.Parallel()

	// One row per agent by default: a list of every version an agent ever had
	// answers a question nobody opened the screen to ask.
	agents := &fakeAgents{}
	if _, err := NewServer(ledger.NewMemory(), "test").WithAgents(agents).
		ListAgents(as(domain.RoleAuthor), openapi.ListAgentsRequestObject{}); err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if agents.allAsked {
		t.Error("the default asked for every version")
	}
}

func TestListAgents_companyWideGrantSeesItsAreas(t *testing.T) {
	t.Parallel()

	// Comparing scopes exactly instead of by containment made the first
	// administrator of an installation — granted across the company — see
	// nothing inside it.
	agents := &fakeAgents{published: []domain.AgentSummary{
		{ID: "triage", Scope: domain.Scope{Company: "acme", Area: "cx"}},
		{ID: "leads", Scope: domain.Scope{Company: "other", Area: "cx"}},
	}}

	caller := auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme"}, Role: domain.RoleCurator}},
	})

	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(agents).
		ListAgents(caller, openapi.ListAgentsRequestObject{})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	body := resp.(openapi.ListAgents200JSONResponse)
	if len(body.Items) != 1 || body.Items[0].AgentId != "triage" {
		t.Fatalf("items = %v, want the agent inside the granted company and nothing else", body.Items)
	}
}

// fakeTools stands in for the published catalogue.
type fakeTools struct {
	entries []domain.ToolEntry
	err     error
}

func (f *fakeTools) Tools(context.Context) ([]domain.ToolEntry, error) { return f.entries, f.err }

func TestListApprovals_saysWhatTheActionDoes(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	seedPendingApproval(t, store, "crm.refund")

	tools := &fakeTools{entries: []domain.ToolEntry{
		{ID: "crm.refund", Effect: domain.EffectFinancial},
	}}
	server := NewServer(store, "test").WithAdministration(nil, tools, nil)

	resp, err := server.ListApprovals(as(domain.RoleApprover), openapi.ListApprovalsRequestObject{})
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	page := resp.(openapi.ListApprovals200JSONResponse)
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(page.Items))
	}
	// Deciding on a refund without being told it is a refund is deciding
	// blind, and the approver deliberately does not hold tool:read.
	if page.Items[0].Effect == nil || *page.Items[0].Effect != "financial" {
		t.Errorf("effect = %v, want financial", page.Items[0].Effect)
	}
}

func TestListApprovals_unreadableCatalogueStillShowsTheQueue(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	seedPendingApproval(t, store, "crm.note")

	server := NewServer(store, "test").
		WithAdministration(nil, &fakeTools{err: errors.New("catalogue unavailable")}, nil)

	resp, err := server.ListApprovals(as(domain.RoleApprover), openapi.ListApprovalsRequestObject{})
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	page := resp.(openapi.ListApprovals200JSONResponse)
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want the queue anyway", len(page.Items))
	}
	// Absent rather than guessed: calling an unknown effect "read" would
	// understate what is being asked for.
	if page.Items[0].Effect != nil {
		t.Errorf("effect = %v, want absent when the catalogue could not be read", page.Items[0].Effect)
	}
}

func seedPendingApproval(t *testing.T, store *ledger.Memory, tool string) {
	t.Helper()
	ctx := context.Background()

	base := domain.Step{
		RunID: "run-1", Scope: adminScope, AgentID: "triage", VersionID: "v1",
		At: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
	}
	base.Kind = domain.StepRunStarted
	if _, err := store.Append(ctx, base); err != nil {
		t.Fatalf("seed: %v", err)
	}

	asked := base
	asked.Kind = domain.StepApprovalRequested
	asked.Payload = []byte(`{"tool":"` + tool + `","rule":"taint","reason":"untrusted argument"}`)
	if _, err := store.Append(ctx, asked); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestDecideApproval_withoutThePermission_isRefused(t *testing.T) {
	t.Parallel()

	// Deciding was reachable by any authenticated caller. A curator writes the
	// rules; approving exceptions to them is a different grant, and that
	// separation is the whole reason the roles are separate.
	store := ledger.NewMemory()
	seedPendingApproval(t, store, "crm.note")

	resp, err := NewServer(store, "test").DecideApproval(as(domain.RoleCurator),
		openapi.DecideApprovalRequestObject{
			RunId: "run-1",
			Body:  &openapi.DecideApprovalJSONRequestBody{Approved: true, AtSeq: 2},
		})
	if err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}
	if _, refused := resp.(openapi.DecideApproval403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
}

func TestListApprovals_showsOnlyTheQueuesTheCallerMayActOn(t *testing.T) {
	t.Parallel()

	// Reading somebody else's queue tells you which actions an agent proposed
	// in an area you have no part in (PRD NF-06).
	store := ledger.NewMemory()
	seedPendingApproval(t, store, "crm.note")

	resp, err := NewServer(store, "test").ListApprovals(inArea("marketing", domain.RoleApprover),
		openapi.ListApprovalsRequestObject{})
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	page := resp.(openapi.ListApprovals200JSONResponse)
	if len(page.Items) != 0 {
		t.Errorf("items = %v, want nothing outside the caller's scope", page.Items)
	}
}

// fakeCeilings stands in for the configured scope budgets.
type fakeCeilings struct {
	configured []domain.ScopeBudget
	put        domain.ScopeBudget
	putBy      domain.UserID
}

func (f *fakeCeilings) List(context.Context) ([]domain.ScopeBudget, error) {
	return f.configured, nil
}
func (f *fakeCeilings) Put(_ context.Context, by domain.UserID, b domain.ScopeBudget) error {
	f.put, f.putBy = b, by
	return nil
}
func (f *fakeCeilings) Delete(context.Context, domain.UserID, domain.Scope) error { return nil }

func TestPutBudget_readsTheThreeScopeShapes(t *testing.T) {
	t.Parallel()

	for raw, want := range map[string]domain.Scope{
		"installation": {},
		"acme":         {Company: "acme"},
		"acme/cx":      {Company: "acme", Area: "cx"},
	} {
		ceilings := &fakeCeilings{}
		_, err := NewServer(ledger.NewMemory(), "test").WithCeilings(ceilings).
			PutBudget(as(domain.RoleCurator), openapi.PutBudgetRequestObject{
				Scope: raw,
				Body:  &openapi.PutBudgetJSONRequestBody{Period: "monthly", Micros: ptr(int64(500))},
			})
		if err != nil {
			t.Fatalf("PutBudget(%q): %v", raw, err)
		}
		if ceilings.put.Scope != want {
			t.Errorf("scope for %q = %v, want %v", raw, ceilings.put.Scope, want)
		}
	}
}

func TestPutBudget_anAreaWithNoCompanyIsNotAScope(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").WithCeilings(&fakeCeilings{}).
		PutBudget(as(domain.RoleCurator), openapi.PutBudgetRequestObject{
			Scope: "/cx",
			Body:  &openapi.PutBudgetJSONRequestBody{Period: "monthly"},
		})
	if err != nil {
		t.Fatalf("PutBudget: %v", err)
	}
	if _, bad := resp.(openapi.PutBudget400ApplicationProblemPlusJSONResponse); !bad {
		t.Fatalf("response = %T, want 400", resp)
	}
}

func TestPutBudget_withoutThePermission_isRefused(t *testing.T) {
	t.Parallel()

	// Raising a ceiling is how an expensive month happens; it is the Curator's
	// act, not an approver's.
	ceilings := &fakeCeilings{}
	resp, err := NewServer(ledger.NewMemory(), "test").WithCeilings(ceilings).
		PutBudget(as(domain.RoleApprover), openapi.PutBudgetRequestObject{
			Scope: "acme/cx",
			Body:  &openapi.PutBudgetJSONRequestBody{Period: "monthly", Micros: ptr(int64(1))},
		})
	if err != nil {
		t.Fatalf("PutBudget: %v", err)
	}
	if _, refused := resp.(openapi.PutBudget403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
	if ceilings.put.Period != "" {
		t.Error("a refused request still reached the store")
	}
}

func TestListIntegrations_dropsAServerNoWorkerStillHolds(t *testing.T) {
	t.Parallel()

	// Workers restate what they hold on every pass, so an observation that
	// stopped being refreshed is the ghost of a process that is gone. It is
	// not configured, so it cannot be edited or removed — leaving it on the
	// screen leaves a row nobody can get rid of.
	admin := &fakeAdmin{}
	health := fakeHealth{"antigo": {
		Name: "antigo", Reachable: true,
		ObservedAt: time.Now().Add(-time.Hour), ObservedBy: "worker-morto",
	}}

	resp, err := serverWith(t, admin).WithHealth(health).
		ListIntegrations(as(domain.RoleCurator), openapi.ListIntegrationsRequestObject{})
	if err != nil {
		t.Fatalf("ListIntegrations: %v", err)
	}
	body := resp.(openapi.ListIntegrations200JSONResponse)
	if len(body.McpServers) != 0 {
		t.Errorf("servers = %+v, want the ghost gone", body.McpServers)
	}
}

func TestListIntegrations_keepsOneAWorkerStillHolds(t *testing.T) {
	t.Parallel()

	// The screen answers what the installation talks to, and a server passed
	// by flag is part of that however little the console can do about it.
	health := fakeHealth{"crm": {
		Name: "crm", Reachable: true, ToolCount: 3,
		ObservedAt: time.Now(), ObservedBy: "worker-1",
	}}

	resp, err := serverWith(t, &fakeAdmin{}).WithHealth(health).
		ListIntegrations(as(domain.RoleCurator), openapi.ListIntegrationsRequestObject{})
	if err != nil {
		t.Fatalf("ListIntegrations: %v", err)
	}
	body := resp.(openapi.ListIntegrations200JSONResponse)
	if len(body.McpServers) != 1 || body.McpServers[0].Name != "crm" {
		t.Fatalf("servers = %+v", body.McpServers)
	}
	if managed := body.McpServers[0].Managed; managed == nil || *managed {
		t.Error("a server nobody configured here is reported as managed")
	}
}

// fakeHealth stands in for what workers observed.
type fakeHealth map[string]domain.IntegrationHealth

func (f fakeHealth) All(context.Context) (map[string]domain.IntegrationHealth, error) {
	return f, nil
}

func TestListTools_aServerThatStoppedAnswering_marksItsToolsUnoffered(t *testing.T) {
	t.Parallel()

	// The published list never shrinks: two workers connected to different
	// servers would delete each other's rows if it did. So a tool from a
	// server nobody can reach stays listed — and a screen that did not say so
	// would be offering a capability nothing has.
	server := NewServer(ledger.NewMemory(), "test").
		WithAdministration(nil, &fakeTools{entries: []domain.ToolEntry{
			{ID: "crm.lookup", Server: "crm", Effect: domain.EffectRead},
			{ID: "old.thing", Server: "removed", Effect: domain.EffectRead},
		}}, nil).
		WithHealth(fakeHealth{"crm": {Name: "crm", Reachable: true, ObservedAt: time.Now()}})

	got, err := server.ListTools(as(domain.RoleCurator), openapi.ListToolsRequestObject{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	listed := got.(openapi.ListTools200JSONResponse)

	offered := map[string]bool{}
	for _, tool := range listed.Items {
		offered[tool.ToolId] = tool.Offered != nil && *tool.Offered
	}
	if !offered["crm.lookup"] {
		t.Error("a tool from a server that answers reads as unoffered")
	}
	if offered["old.thing"] {
		t.Error("a tool from a server nobody observed reads as offered")
	}
}

func TestListTools_aStaleObservation_isSilenceRatherThanAYes(t *testing.T) {
	t.Parallel()

	// A worker that stopped observing leaves its last reading behind. Trusting
	// it for ever would report a server as answering years after it stopped.
	server := NewServer(ledger.NewMemory(), "test").
		WithAdministration(nil, &fakeTools{entries: []domain.ToolEntry{
			{ID: "crm.lookup", Server: "crm", Effect: domain.EffectRead},
		}}, nil).
		WithHealth(fakeHealth{"crm": {
			Name: "crm", Reachable: true,
			ObservedAt: time.Now().Add(-time.Hour),
		}})

	got, err := server.ListTools(as(domain.RoleCurator), openapi.ListToolsRequestObject{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	listed := got.(openapi.ListTools200JSONResponse)

	if len(listed.Items) != 1 || (listed.Items[0].Offered != nil && *listed.Items[0].Offered) {
		t.Errorf("items = %+v, want the stale server's tool unoffered", listed.Items)
	}
}

/*
The Curator reads the reasoning before confirming.

The shipped catalogue exists so that registering a well-known server is one
confirmation rather than forty rulings invented from a list of bare names. A
suggestion the screen never receives keeps none of that promise — the data
would be in the binary and the Curator would still be guessing.

Resolved on read rather than stored. It is derived from a table shipped in the
binary, and a derived value persisted is one that goes stale against the table
it came from.
*/
func TestListTools_aKnownServer_carriesTheSuggestionAndItsReason(t *testing.T) {
	t.Parallel()
	shipped, err := known.Load()
	if err != nil {
		t.Fatalf("known: %v", err)
	}

	s := NewServer(ledger.NewMemory(), "test").
		WithAdministration(nil, &fakeTools{entries: []domain.ToolEntry{
			{ID: "github.merge_pull_request", Server: "github", Effect: domain.EffectUnknown},
		}}, nil).
		WithKnown(shipped)

	resp, err := s.ListTools(everywhere(domain.RoleCurator), openapi.ListToolsRequestObject{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	page, ok := resp.(openapi.ListTools200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the catalogue", resp)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %+v, want the one tool", page.Items)
	}

	got := page.Items[0]
	if got.Effect != openapi.ToolEffectUnknown {
		t.Errorf("effect = %q, want it still unclassified", got.Effect)
	}
	if got.Suggested == nil {
		t.Fatal("no suggestion reached the screen")
	}
	if got.Suggested.Effect != openapi.EffectDestructive {
		t.Errorf("suggested = %q, want destructive", got.Suggested.Effect)
	}
	if got.Suggested.Why == "" {
		t.Error("a suggestion with no reasoning is a number to click past")
	}
}
