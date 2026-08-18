package admin_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

func newIntegrations(t *testing.T) *admin.Integrations {
	t.Helper()

	pool := openPool(t)
	if _, err := pool.Exec(context.Background(),
		`delete from settings where kind in ('mcp_server', 'mcp_probe', 'model_provider', 'model_price')`); err != nil {
		t.Fatalf("clean: %v", err)
	}

	key := make([]byte, 32)
	v, err := vault.New(key, "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return admin.NewIntegrations(pool, settings.NewStore(pool, v))
}

func TestPutProvider_theCredentialNeverLeavesTheVaultOnAnOrdinaryRead(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()

	if err := i.PutProvider(ctx, "usr_ana", platform,
		domain.ModelProvider{Name: "openai", Kind: "openai_compatible", BaseURL: "https://api.openai.com/v1", Enabled: true},
		"sk-secret"); err != nil {
		t.Fatalf("PutProvider: %v", err)
	}

	providers, err := i.Providers(ctx)
	if err != nil {
		t.Fatalf("Providers: %v", err)
	}
	if len(providers) != 1 || !providers[0].HasKey {
		t.Fatalf("Providers = %+v, want one provider with a stored key", providers)
	}

	// Listing says a credential exists. Getting it back is a separate act, so
	// the trail can record that somebody asked.
	secret, err := i.Credential(ctx, "openai")
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	if secret != "sk-secret" {
		t.Errorf("Credential = %q, want the sealed key back", secret)
	}
}

func TestPutProvider_withoutAKey_keepsTheStoredOne(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()

	provider := domain.ModelProvider{Name: "openai", Kind: "openai_compatible", BaseURL: "https://a.example", Enabled: true}
	if err := i.PutProvider(ctx, "usr_ana", platform, provider, "sk-secret"); err != nil {
		t.Fatalf("PutProvider: %v", err)
	}

	// Changing a base URL must not demand re-entering the credential: that is
	// how keys end up pasted into chat so somebody can look one up.
	provider.BaseURL = "https://b.example"
	if err := i.PutProvider(ctx, "usr_ana", platform, provider, ""); err != nil {
		t.Fatalf("PutProvider without key: %v", err)
	}

	secret, err := i.Credential(ctx, "openai")
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	if secret != "sk-secret" {
		t.Errorf("Credential = %q, want the original key kept", secret)
	}
}

func TestPutProvider_recordsTheChangeWithoutTheCredential(t *testing.T) {
	i := newIntegrations(t)
	pool := openPool(t)
	ctx := context.Background()

	if err := i.PutProvider(ctx, "usr_ana", platform,
		domain.ModelProvider{Name: "deepseek", Kind: "openai_compatible", BaseURL: "https://api.deepseek.com", Enabled: true},
		"sk-do-not-log-me"); err != nil {
		t.Fatalf("PutProvider: %v", err)
	}

	var detail string
	if err := pool.QueryRow(ctx,
		`select detail::text from admin_events where target = 'deepseek' order by event_id desc limit 1`,
	).Scan(&detail); err != nil {
		t.Fatalf("read event: %v", err)
	}

	// The trail records that a key arrived, never the key.
	if want := "sk-do-not-log-me"; contains(detail, want) {
		t.Errorf("the administrative trail contains the credential: %s", detail)
	}
	if !contains(detail, "keyChanged") {
		t.Errorf("detail = %s, want it to record that a credential changed", detail)
	}
}

func TestPutMCPServer_withoutACommand_isRefused(t *testing.T) {
	i := newIntegrations(t)

	// A server row with nothing to run is a tool source that silently offers
	// nothing, which reads as "no tools" rather than "misconfigured".
	err := i.PutMCPServer(context.Background(), "usr_ana", platform, domain.MCPServer{Name: "crm"}, domain.MCPCredentialPatch{})
	if err == nil {
		t.Fatal("PutMCPServer accepted a server with no command")
	}
}

func TestDeleteMCPServer_isRecorded(t *testing.T) {
	i := newIntegrations(t)
	pool := openPool(t)
	ctx := context.Background()

	if err := i.PutMCPServer(ctx, "usr_ana", platform,
		domain.MCPServer{
			Name: "crm", Command: "/usr/local/bin/crm-mcp", Enabled: true,
			AcceptsLocalExecution: true,
		}, domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if err := i.DeleteMCPServer(ctx, "usr_ana", platform, "crm"); err != nil {
		t.Fatalf("DeleteMCPServer: %v", err)
	}

	servers, _ := i.MCPServers(ctx)
	if len(servers) != 0 {
		t.Errorf("MCPServers = %+v, want none", servers)
	}

	// Removing a tool source changes what agents can do. An auditor asking
	// "why did this stop working" needs the moment it was removed.
	var action string
	if err := pool.QueryRow(ctx,
		`select action from admin_events where target = 'crm' order by event_id desc limit 1`).Scan(&action); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if action != "mcp_server.removed" {
		t.Errorf("action = %q, want mcp_server.removed", action)
	}
}

func TestRequestMCPProbe_recordsAWorkerRequestForAnEnabledServer(t *testing.T) {
	i := newIntegrations(t)
	pool := openPool(t)
	ctx := context.Background()

	if err := i.PutMCPServer(ctx, "usr_ana", platform,
		domain.MCPServer{
			Name: "crm", Command: "/usr/local/bin/crm-mcp", Enabled: true,
			AcceptsLocalExecution: true,
		}, domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if err := i.RequestMCPProbe(ctx, "usr_bia", platform, "crm"); err != nil {
		t.Fatalf("RequestMCPProbe: %v", err)
	}

	claimed, err := i.ClaimMCPProbes(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimMCPProbes: %v", err)
	}
	if len(claimed) != 1 || claimed[0] != "crm" {
		t.Fatalf("claimed = %v, want crm", claimed)
	}
	again, err := i.ClaimMCPProbes(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimMCPProbes again: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("claimed again = %v, want the request consumed once", again)
	}

	var action, by string
	if err := pool.QueryRow(ctx,
		`select action, principal_id from admin_events where target = 'crm' order by event_id desc limit 1`,
	).Scan(&action, &by); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if action != "mcp_server.probe_requested" || by != "usr_bia" {
		t.Errorf("event = %s by %s, want the probe request by usr_bia", action, by)
	}
}

func TestRequestMCPProbe_refusesDisabledAndUnknownServers(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()

	if err := i.PutMCPServer(ctx, "usr_ana", platform,
		domain.MCPServer{
			Name: "crm", Command: "/usr/local/bin/crm-mcp", Enabled: false,
			AcceptsLocalExecution: true,
		}, domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	if err := i.RequestMCPProbe(ctx, "usr_bia", platform, "crm"); !errors.Is(err, admin.ErrMCPServerDisabled) {
		t.Fatalf("disabled probe = %v, want %v", err, admin.ErrMCPServerDisabled)
	}
	if err := i.RequestMCPProbe(ctx, "usr_bia", platform, "missing"); !errors.Is(err, admin.ErrMCPServerUnknown) {
		t.Fatalf("unknown probe = %v, want %v", err, admin.ErrMCPServerUnknown)
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func TestPutProvider_anthropicWithoutABaseURL_isAccepted(t *testing.T) {
	i := newIntegrations(t)

	err := i.PutProvider(t.Context(), "usr_a", domain.Scope{},
		domain.ModelProvider{Name: "anthropic", Kind: "anthropic"}, "sk-test")

	// Its client already knows the address, so demanding one asks for a value
	// nobody has — and refusing without it made the reference provider
	// impossible to configure at all.
	if err != nil {
		t.Fatalf("put: %v", err)
	}
}

func TestPutProvider_openAICompatibleWithoutABaseURL_isRefused(t *testing.T) {
	i := newIntegrations(t)

	err := i.PutProvider(t.Context(), "usr_a", domain.Scope{},
		domain.ModelProvider{Name: "vllm", Kind: "openai_compatible"}, "")

	// Here the address genuinely has to come from somewhere: only the
	// installation knows where its own model is listening.
	if !errors.Is(err, admin.ErrNoBaseURL) {
		t.Fatalf("got %v, want ErrNoBaseURL", err)
	}
}

// A tool server is either a process this installation runs or an address it
// calls. Everything here is about not letting one be configured as the other.

func TestPutMCPServer_remote_needsAnAddressRatherThanACommand(t *testing.T) {
	i := newIntegrations(t)

	err := i.PutMCPServer(context.Background(), "usr_ana", platform, domain.MCPServer{
		Name: "github", Transport: domain.TransportHTTP, Enabled: true,
	}, domain.MCPCredentialPatch{})
	if !errors.Is(err, admin.ErrNoURL) {
		t.Fatalf("PutMCPServer = %v, want a refusal naming the missing address", err)
	}
}

func TestPutMCPServer_local_stillNeedsACommand(t *testing.T) {
	i := newIntegrations(t)

	err := i.PutMCPServer(context.Background(), "usr_ana", platform, domain.MCPServer{
		Name: "crm", Transport: domain.TransportStdio, Enabled: true,
	}, domain.MCPCredentialPatch{})
	if !errors.Is(err, admin.ErrNoCommand) {
		t.Fatalf("PutMCPServer = %v", err)
	}
}

func TestPutMCPServer_remote_sealsItsTokenAndNeverListsIt(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()

	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "github", Transport: domain.TransportHTTP,
		URL: "https://api.githubcopilot.com/mcp/", Enabled: true,
	}, domain.MCPCredentialPatch{Token: ptr("ghp_secret")}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if len(servers) != 1 || servers[0].URL != "https://api.githubcopilot.com/mcp/" {
		t.Fatalf("servers = %+v", servers)
	}
	// A remote server's token is a credential like any other: the listing says
	// one exists, and reading it is a separate act.
	if !servers[0].HasSecret {
		t.Error("the listing does not say a token is stored")
	}
	creds, err := i.MCPCredentials(ctx, "github")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.Token != "ghp_secret" {
		t.Errorf("token = %q", creds.Token)
	}
}

func TestPutMCPServer_remote_sealsOAuthAndNeverListsIt(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()

	grant := domain.MCPOAuthGrant{
		AccessToken: "access", RefreshToken: "refresh",
		TokenURL: "https://issuer.example/token", ClientID: "client",
		ClientSecret: "secret", Scopes: []string{"sheets.readonly"},
	}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "google-workspace", Transport: domain.TransportHTTP,
		URL: "https://mcp.example.com/google", Enabled: true,
	}, domain.MCPCredentialPatch{OAuth: &grant}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if !servers[0].HasSecret || !servers[0].HasOAuth {
		t.Fatalf("server = %+v, want only presence of the oauth grant", servers[0])
	}
	creds, err := i.MCPCredentials(ctx, "google-workspace")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.OAuth == nil || creds.OAuth.AccessToken != "access" ||
		creds.OAuth.RefreshToken != "refresh" ||
		creds.OAuth.ClientSecret != "secret" {
		t.Fatalf("oauth = %+v, want it back only through the credential reader", creds.OAuth)
	}
}

func TestPutMCPServer_remote_sealsHeadersAndNeverListsThem(t *testing.T) {
	i := newIntegrations(t)
	pool := openPool(t)
	ctx := context.Background()

	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "newrelic", Transport: domain.TransportHTTP,
		URL: "https://mcp.newrelic.com/mcp/", Enabled: true,
	}, domain.MCPCredentialPatch{
		Headers: map[string]string{"Api-Key": "nr_secret"},
	}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if !servers[0].HasSecret || servers[0].HasOAuth {
		t.Fatalf("server = %+v, want only presence of a sealed remote credential", servers[0])
	}
	creds, err := i.MCPCredentials(ctx, "newrelic")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.Headers["Api-Key"] != "nr_secret" || creds.Token != "" || creds.OAuth != nil {
		t.Fatalf("credentials = %+v, want only the custom header", creds)
	}

	var detail string
	if err := pool.QueryRow(ctx,
		`select detail::text from admin_events where target = 'newrelic' order by event_id desc limit 1`,
	).Scan(&detail); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if !contains(detail, "headersChanged") {
		t.Errorf("detail = %s, want it to record that header credentials changed", detail)
	}
	if contains(detail, "nr_secret") {
		t.Errorf("the administrative trail contains the header credential: %s", detail)
	}
}

func TestPutMCPServer_writingOAuthReplacesStoredBearer(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "remote", Transport: domain.TransportHTTP,
		URL: "https://tools.example.com/mcp", Enabled: true,
	}

	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{Token: ptr("ghp_stale")}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{OAuth: &domain.MCPOAuthGrant{AccessToken: "access"}}); err != nil {
		t.Fatalf("PutMCPServer oauth: %v", err)
	}

	creds, err := i.MCPCredentials(ctx, "remote")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.Token != "" || creds.OAuth == nil {
		t.Fatalf("creds = %+v, want oauth to replace the bearer", creds)
	}
}

func TestPutMCPServer_writingBearerReplacesStoredOAuth(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "remote", Transport: domain.TransportHTTP,
		URL: "https://tools.example.com/mcp", Enabled: true,
	}

	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{OAuth: &domain.MCPOAuthGrant{AccessToken: "access"}}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{Token: ptr("ghp_new")}); err != nil {
		t.Fatalf("PutMCPServer bearer: %v", err)
	}

	creds, err := i.MCPCredentials(ctx, "remote")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.Token != "ghp_new" || creds.OAuth != nil {
		t.Fatalf("creds = %+v, want bearer to replace oauth", creds)
	}
	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if servers[0].HasOAuth {
		t.Fatalf("server = %+v, want oauth presence cleared", servers[0])
	}
}

func TestRefreshMCPServerOAuth_persistsTheRotatedGrantAndRecordsNoSecret(t *testing.T) {
	i := newIntegrations(t)
	pool := openPool(t)
	ctx := context.Background()
	surface := []string{"read_sheet"}
	server := domain.MCPServer{
		Name: "google-sheets", Transport: domain.TransportHTTP,
		URL: "https://mcp.example.com/google", Enabled: true,
		Surface: &surface,
	}
	was := domain.MCPOAuthGrant{
		AccessToken: "old-access", RefreshToken: "old-refresh",
		TokenURL: "https://issuer.example/token", ClientID: "client",
		ClientSecret: "secret", ExpiresAtUnix: 100, Scopes: []string{"old"},
	}
	next := was
	next.AccessToken = "new-access"
	next.RefreshToken = "new-refresh"
	next.ExpiresAtUnix = 200
	next.Scopes = []string{"new"}

	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{OAuth: &was}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if err := i.RefreshMCPServerOAuth(ctx, "google-sheets", was, next); err != nil {
		t.Fatalf("RefreshMCPServerOAuth: %v", err)
	}

	creds, err := i.MCPCredentials(ctx, "google-sheets")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.OAuth == nil ||
		creds.OAuth.AccessToken != "new-access" ||
		creds.OAuth.RefreshToken != "new-refresh" ||
		creds.OAuth.ClientSecret != "secret" ||
		creds.OAuth.Scopes[0] != "new" {
		t.Fatalf("oauth = %+v, want the refreshed grant persisted", creds.OAuth)
	}
	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if servers[0].Surface == nil || len(*servers[0].Surface) != 1 || (*servers[0].Surface)[0] != "read_sheet" {
		t.Fatalf("surface = %v; persisting oauth rewrote the server value", servers[0].Surface)
	}

	var principal, detail string
	if err := pool.QueryRow(ctx, `
		select principal_id, detail::text from admin_events
		where action = 'mcp_oauth.refreshed' and target = 'google-sheets'
		order by event_id desc limit 1`).Scan(&principal, &detail); err != nil {
		t.Fatalf("read the trail: %v", err)
	}
	if principal != string(domain.SystemWorker) {
		t.Fatalf("principal = %q, want the worker principal", principal)
	}
	for _, secret := range []string{"old-access", "new-access", "old-refresh", "new-refresh", "secret"} {
		if strings.Contains(detail, secret) {
			t.Fatalf("detail leaked %q: %s", secret, detail)
		}
	}
	for _, flag := range []string{"accessTokenChanged", "refreshTokenChanged", "expiresAtChanged", "scopesChanged"} {
		if !strings.Contains(detail, flag) {
			t.Fatalf("detail = %s, want %s recorded", detail, flag)
		}
	}
}

func TestRefreshMCPServerOAuth_refusesToOverwriteAChangedGrant(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "google-sheets", Transport: domain.TransportHTTP,
		URL: "https://mcp.example.com/google", Enabled: true,
	}
	was := domain.MCPOAuthGrant{AccessToken: "old-access", RefreshToken: "old-refresh", TokenURL: "https://issuer.example/token"}
	replaced := domain.MCPOAuthGrant{AccessToken: "operator-access", RefreshToken: "operator-refresh", TokenURL: "https://issuer.example/token"}
	next := was
	next.AccessToken = "new-access"
	next.RefreshToken = "new-refresh"

	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{OAuth: &was}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{OAuth: &replaced}); err != nil {
		t.Fatalf("PutMCPServer replacing: %v", err)
	}

	err := i.RefreshMCPServerOAuth(ctx, "google-sheets", was, next)
	if !errors.Is(err, admin.ErrOAuthGrantChanged) {
		t.Fatalf("RefreshMCPServerOAuth = %v, want the changed grant refused", err)
	}
	creds, err := i.MCPCredentials(ctx, "google-sheets")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.OAuth == nil || creds.OAuth.RefreshToken != "operator-refresh" {
		t.Fatalf("oauth = %+v, want the operator's newer grant preserved", creds.OAuth)
	}
}

func TestPutMCPServer_storedBeforeTransportsExisted_readsAsLocal(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()

	// Rows written before the field existed carry no transport. They are the
	// commands they always were, and defaulting them to anything else would
	// silently stop an installation's tools from connecting on upgrade.
	// Accepted here because this test is about how an absent transport reads,
	// not about who agreed to local execution. The two are separate rules and
	// a fixture that conflated them would fail for the wrong reason.
	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "crm", Command: "bin/devstack", Args: []string{"mcp"}, Enabled: true,
		AcceptsLocalExecution: true,
	}, domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if servers[0].Transport != domain.TransportStdio {
		t.Errorf("transport = %q, want it read as a local command", servers[0].Transport)
	}
}

/*
A local server is not configured by accident.

stdio is a program this installation starts inside the worker: it runs as the
worker, on its filesystem, from inside its network. Nothing about the transport
field says that, and a form that offered it beside a URL as if the two were the
same kind of choice would be the platform failing to mention the difference.

Refused rather than warned. A warning is read by whoever was already careful.
*/
func TestPutMCPServer_aLocalServerNobodyAccepted_isRefused(t *testing.T) {
	store := newIntegrations(t)

	err := store.PutMCPServer(context.Background(), "usr_ana", domain.Scope{},
		domain.MCPServer{
			Name: "local-tools", Transport: domain.TransportStdio,
			Command: "/usr/bin/mcp-server", Enabled: true,
		}, domain.MCPCredentialPatch{})
	if !errors.Is(err, admin.ErrLocalExecutionNotAccepted) {
		t.Fatalf("err = %v, want the acceptance to be required", err)
	}
}

// And accepted, it is recorded — with who accepted, because the acceptance is
// a person's and not a checkbox's.
func TestPutMCPServer_anAcceptedLocalServer_recordsWhoAcceptedIt(t *testing.T) {
	store := newIntegrations(t)
	ctx := context.Background()

	if err := store.PutMCPServer(ctx, "usr_ana", domain.Scope{}, domain.MCPServer{
		Name: "local-tools", Transport: domain.TransportStdio,
		Command: "/usr/bin/mcp-server", Enabled: true,
		AcceptsLocalExecution: true,
	}, domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	servers, err := store.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	var found *domain.MCPServer
	for i := range servers {
		if servers[i].Name == "local-tools" {
			found = &servers[i]
		}
	}
	if found == nil {
		t.Fatalf("servers = %+v, want the one just written", servers)
	}
	if !found.AcceptsLocalExecution {
		t.Error("read back unaccepted; the worker would refuse to start a server the screen shows as configured")
	}
}

// An HTTP server needs no such acceptance. The platform sends it a request; it
// starts nothing, and demanding a decision about local execution for a URL
// would teach people to tick the box without reading it.
func TestPutMCPServer_aRemoteServer_needsNoAcceptance(t *testing.T) {
	store := newIntegrations(t)

	if err := store.PutMCPServer(context.Background(), "usr_ana", domain.Scope{},
		domain.MCPServer{
			Name: "remote-tools", Transport: domain.TransportHTTP,
			URL: "https://tools.example.com/mcp", Enabled: true,
		}, domain.MCPCredentialPatch{Token: ptr("tok")}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
}

/*
A local server receives its credential explicitly, or not at all.

The worker stopped handing children its own environment, which closed a hole
and removed the only way a local server ever got a token. This is the
replacement: variables of its own, sealed in the vault beside the bearer, given
to the process and to nothing else.
*/
func TestPutMCPServer_local_sealsItsVariablesAndNeverListsThem(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()

	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "local-github", Transport: domain.TransportStdio,
		Command: "/usr/bin/mcp-github", Enabled: true, AcceptsLocalExecution: true,
	}, domain.MCPCredentialPatch{Env: map[string]string{"GITHUB_TOKEN": "ghp_secret"}}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if !servers[0].HasSecret {
		t.Error("the listing does not say a credential is stored")
	}
	if !servers[0].HasVariables {
		t.Error("the listing does not say variables are stored")
	}
	creds, err := i.MCPCredentials(ctx, "local-github")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.Env["GITHUB_TOKEN"] != "ghp_secret" {
		t.Errorf("env = %v, want the variable back from the vault", creds.Env)
	}
}

/*
A managed config file is stored like a credential and listed only by presence.

The content may be a DSN, service-account grant or connection profile, so the
screen gets enough to offer "remove it" and never enough to print it back.
*/
func TestPutMCPServer_local_sealsAConfigFileAndOnlyListsPresence(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	env := "TOOLBOX_CONFIG"

	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "toolbox", Transport: domain.TransportStdio,
		Command: "/usr/bin/toolbox", Enabled: true, AcceptsLocalExecution: true,
		ConfigFileEnv: &env,
	}, domain.MCPCredentialPatch{ConfigFile: ptr("sources:\n- kind: postgres\n")}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if !servers[0].HasSecret || !servers[0].HasConfigFile {
		t.Fatalf("server = %+v, want a sealed document and config-file presence", servers[0])
	}
	if servers[0].HasVariables {
		t.Fatalf("server = %+v, a config file alone should not read as variables", servers[0])
	}
	if servers[0].ConfigFileEnv == nil || *servers[0].ConfigFileEnv != "TOOLBOX_CONFIG" {
		t.Fatalf("config env = %v, want the public path variable returned", servers[0].ConfigFileEnv)
	}
	creds, err := i.MCPCredentials(ctx, "toolbox")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.ConfigFile != "sources:\n- kind: postgres\n" {
		t.Errorf("config file = %q, want it back only through the credential reader", creds.ConfigFile)
	}
}

/*
Correcting one thing does not erase another.

The failure is quiet and expensive: somebody fixes a command, the token they
never had to hand goes with it, and the server stops answering for a reason
nothing on the screen mentions.
*/
func TestPutMCPServer_aWriteThatOmitsTheCredential_keepsTheStoredOne(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "local-github", Transport: domain.TransportStdio,
		Command: "/usr/bin/mcp-github", Enabled: true, AcceptsLocalExecution: true,
	}

	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{Env: map[string]string{"GITHUB_TOKEN": "ghp_secret"}}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	server.Args = []string{"--verbose"}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer again: %v", err)
	}

	creds, err := i.MCPCredentials(ctx, "local-github")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.Env["GITHUB_TOKEN"] != "ghp_secret" {
		t.Errorf("env = %v; editing the arguments dropped the credential", creds.Env)
	}
}

func TestPutMCPServer_aWriteThatOmitsTheConfigFile_keepsTheStoredOne(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "toolbox", Transport: domain.TransportStdio,
		Command: "/usr/bin/toolbox", Enabled: true, AcceptsLocalExecution: true,
	}

	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{ConfigFile: ptr("sources: []\n")}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	server.Args = []string{"serve"}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer again: %v", err)
	}

	creds, err := i.MCPCredentials(ctx, "toolbox")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.ConfigFile != "sources: []\n" {
		t.Errorf("config file = %q; editing arguments dropped it", creds.ConfigFile)
	}
}

func ptr[T any](v T) *T { return &v }

/*
A credential can be taken back.

The store's own rule is that an omitted secret keeps the stored one, which is
right and, alone, makes a credential impossible to remove: "clear it" and "do
not mention it" arrive at the database as the same write. One of those is
somebody revoking a token.
*/
func TestPutMCPServer_clearingTheToken_actuallyRemovesIt(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "remote", Transport: domain.TransportHTTP,
		URL: "https://tools.example.com/mcp", Enabled: true,
	}

	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{Token: ptr("ghp_secret")}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{Token: ptr("")}); err != nil {
		t.Fatalf("PutMCPServer clearing: %v", err)
	}

	creds, err := i.MCPCredentials(ctx, "remote")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.Token != "" {
		t.Errorf("token = %q; a revoked credential is still in the vault", creds.Token)
	}
	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if servers[0].HasSecret {
		t.Error("the listing still says a credential is stored")
	}
}

func TestPutMCPServer_clearingOAuth_actuallyRemovesIt(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "remote", Transport: domain.TransportHTTP,
		URL: "https://tools.example.com/mcp", Enabled: true,
	}

	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{OAuth: &domain.MCPOAuthGrant{AccessToken: "access"}}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{OAuth: &domain.MCPOAuthGrant{}}); err != nil {
		t.Fatalf("PutMCPServer clearing: %v", err)
	}

	creds, err := i.MCPCredentials(ctx, "remote")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.OAuth != nil {
		t.Fatalf("oauth = %+v; a revoked grant is still in the vault", creds.OAuth)
	}
	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if servers[0].HasSecret || servers[0].HasOAuth {
		t.Fatalf("server = %+v, want no stored credential", servers[0])
	}
}

func TestPutMCPServer_clearingTheConfigFile_removesOnlyThatFile(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "toolbox", Transport: domain.TransportStdio,
		Command: "/usr/bin/toolbox", Enabled: true, AcceptsLocalExecution: true,
	}

	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{
			Env:        map[string]string{"TOKEN": "still-needed"},
			ConfigFile: ptr("sources: []\n"),
		}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{ConfigFile: ptr("")}); err != nil {
		t.Fatalf("PutMCPServer clearing: %v", err)
	}

	creds, err := i.MCPCredentials(ctx, "toolbox")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.ConfigFile != "" {
		t.Errorf("config file = %q; a revoked file is still in the vault", creds.ConfigFile)
	}
	if creds.Env["TOKEN"] != "still-needed" {
		t.Errorf("env = %v; clearing the file dropped the variables", creds.Env)
	}
	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if servers[0].HasConfigFile {
		t.Error("the listing still says a config file is stored")
	}
	if !servers[0].HasSecret {
		t.Error("the listing forgot the remaining variables")
	}
}

func TestPutMCPServer_badConfigFileEnv_isRefused(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	env := "not a variable"

	err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "toolbox", Transport: domain.TransportStdio,
		Command: "/usr/bin/toolbox", Enabled: true, AcceptsLocalExecution: true,
		ConfigFileEnv: &env,
	}, domain.MCPCredentialPatch{ConfigFile: ptr("sources: []\n")})
	if !errors.Is(err, admin.ErrBadConfigFileEnv) {
		t.Fatalf("PutMCPServer = %v, want config-file env refusal", err)
	}
}

/*
Switching transport leaves nothing behind for the shape that cannot use it.

A bearer sealed for a server the platform now starts is material nobody can
see, nobody can send and nobody remembers to revoke.
*/
func TestPutMCPServer_switchingToLocal_dropsTheBearerItCanNoLongerSend(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()

	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "github", Transport: domain.TransportHTTP,
		URL: "https://api.example.com/mcp", Enabled: true,
	}, domain.MCPCredentialPatch{Token: ptr("ghp_secret")}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "github", Transport: domain.TransportStdio,
		Command: "/usr/bin/mcp-github", Enabled: true, AcceptsLocalExecution: true,
	}, domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer switching: %v", err)
	}

	creds, err := i.MCPCredentials(ctx, "github")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.Token != "" {
		t.Errorf("token = %q survived a switch to a shape that cannot send it", creds.Token)
	}
}

func TestPutMCPServer_switchingToLocal_dropsOAuthItCanNoLongerSend(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()

	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "github", Transport: domain.TransportHTTP,
		URL: "https://api.example.com/mcp", Enabled: true,
	}, domain.MCPCredentialPatch{OAuth: &domain.MCPOAuthGrant{AccessToken: "access"}}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "github", Transport: domain.TransportStdio,
		Command: "/usr/bin/mcp-github", Enabled: true, AcceptsLocalExecution: true,
	}, domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer switching: %v", err)
	}

	creds, err := i.MCPCredentials(ctx, "github")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.OAuth != nil {
		t.Fatalf("oauth = %+v survived a switch to a shape that cannot send it", creds.OAuth)
	}
	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if servers[0].HasOAuth {
		t.Fatalf("server = %+v, want oauth presence cleared", servers[0])
	}
}

func TestPutMCPServer_switchingToRemote_dropsTheConfigFileItCanNoLongerMaterialize(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()

	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "toolbox", Transport: domain.TransportStdio,
		Command: "/usr/bin/toolbox", Enabled: true, AcceptsLocalExecution: true,
	}, domain.MCPCredentialPatch{ConfigFile: ptr("sources: []\n")}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "toolbox", Transport: domain.TransportHTTP,
		URL: "https://tools.example.com/mcp", Enabled: true,
	}, domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer switching: %v", err)
	}

	creds, err := i.MCPCredentials(ctx, "toolbox")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.ConfigFile != "" {
		t.Errorf("config file survived a switch to a shape that cannot use it: %q", creds.ConfigFile)
	}
	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if servers[0].HasConfigFile || servers[0].ConfigFileEnv != nil {
		t.Errorf("server = %+v, want no config-file metadata after switching to HTTP", servers[0])
	}
}

/*
Editing anything else does not reopen the tools.

A write that says nothing about the surface is not a request to forget it, and
forgetting reads as "nobody has chosen" — which the runtime treats as every
tool this server offers. So saving a token, correcting a command or switching a
server off would silently widen what agents can reach, which is the one
direction this must never fail in.

Both stored shapes, because the empty one is the one a careless merge loses:
"chosen, and none of it" is a decision, and turning it into "nobody has chosen"
turns a closed server into an open one.
*/
func TestPutMCPServer_aWriteThatOmitsTheSurface_keepsTheStoredOne(t *testing.T) {
	for _, one := range []struct {
		name    string
		surface []string
	}{
		{name: "narrowed", surface: []string{"lookup"}},
		{name: "emptied", surface: []string{}},
	} {
		t.Run(one.name, func(t *testing.T) {
			i := newIntegrations(t)
			ctx := context.Background()
			server := domain.MCPServer{
				Name: one.name, Transport: domain.TransportHTTP,
				URL: "https://tools.example.com/mcp", Enabled: true,
				Surface: &one.surface,
			}

			if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
				domain.MCPCredentialPatch{Token: ptr("tok")}); err != nil {
				t.Fatalf("PutMCPServer: %v", err)
			}

			// The same server again, saying nothing about the surface — which
			// is what saving a credential looks like from here.
			server.Surface = nil
			if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
				domain.MCPCredentialPatch{Token: ptr("rotated")}); err != nil {
				t.Fatalf("PutMCPServer again: %v", err)
			}

			servers, err := i.MCPServers(ctx)
			if err != nil {
				t.Fatalf("MCPServers: %v", err)
			}
			var found *domain.MCPServer
			for k := range servers {
				if servers[k].Name == one.name {
					found = &servers[k]
				}
			}
			if found == nil || found.Surface == nil {
				t.Fatalf("surface = %v; a credential edit reopened every tool", found)
			}
			if len(*found.Surface) != len(one.surface) {
				t.Errorf("surface = %v, want the stored %v", *found.Surface, one.surface)
			}
		})
	}
}

// And a write that names one replaces it, empty included: this is the field
// the page exists to set.
func TestPutMCPServer_aWriteThatNamesTheSurface_replacesIt(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	wide := []string{"lookup", "delete_account"}
	server := domain.MCPServer{
		Name: "crm", Transport: domain.TransportHTTP,
		URL: "https://tools.example.com/mcp", Enabled: true, Surface: &wide,
	}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	narrow := []string{"lookup"}
	server.Surface = &narrow
	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer again: %v", err)
	}

	servers, _ := i.MCPServers(ctx)
	if servers[0].Surface == nil || len(*servers[0].Surface) != 1 {
		t.Errorf("surface = %v, want the narrowed choice", servers[0].Surface)
	}
}

/*
Two people editing one server do not undo each other.

Reading what is stored before the transaction is a lost update waiting for
exactly this: one narrows the surface, the other saves a token having read the
older value, and whichever commits second writes the older value back. The
sequential case passes either way, which is what makes this worth its own test.

The direction is what matters. A surface restored to "nobody has chosen"
reopens every tool the server offers, and it happens on the most ordinary edit
there is.
*/
func TestPutMCPServer_twoWritersAtOnce_doNotRestoreEachOthersOldValues(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	wide := []string{"lookup", "delete_account"}
	server := domain.MCPServer{
		Name: "crm", Transport: domain.TransportHTTP,
		URL: "https://tools.example.com/mcp", Enabled: true, Surface: &wide,
	}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{Token: ptr("first")}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	// One narrows the surface and says nothing about the token; the other
	// rotates the token and says nothing about the surface.
	narrow := []string{"lookup"}
	narrowing := server
	narrowing.Surface = &narrow
	rotating := server
	rotating.Surface = nil

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = i.PutMCPServer(ctx, "usr_ana", platform, narrowing,
			domain.MCPCredentialPatch{})
	}()
	go func() {
		defer wg.Done()
		errs[1] = i.PutMCPServer(ctx, "usr_bob", platform, rotating,
			domain.MCPCredentialPatch{Token: ptr("rotated")})
	}()
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("PutMCPServer: %v", err)
		}
	}

	servers, err := i.MCPServers(ctx)
	if err != nil {
		t.Fatalf("MCPServers: %v", err)
	}
	if servers[0].Surface == nil || len(*servers[0].Surface) != 1 {
		t.Errorf("surface = %v; a concurrent token write reopened the tools",
			servers[0].Surface)
	}
	creds, err := i.MCPCredentials(ctx, "crm")
	if err != nil {
		t.Fatalf("MCPCredentials: %v", err)
	}
	if creds.Token != "rotated" {
		t.Errorf("token = %q; a concurrent surface write restored the old one", creds.Token)
	}
}

/*
The trail keeps the shape it has always had.

An administrative record is read by queries written against the field names it
used at the time. Renaming one does not break a runtime and does break the
trail: the older half stops answering the query that reads the newer half, and
nobody notices until somebody is trying to establish what happened.

`transport` and not `kind`, because `kind` is the provider's word for a
different thing entirely.
*/
func TestPutMCPServer_recordsTheTransportUnderTheNameTheTrailAlwaysUsed(t *testing.T) {
	i := newIntegrations(t)
	pool := openPool(t)
	ctx := context.Background()

	if err := i.PutMCPServer(ctx, "usr_ana", platform, domain.MCPServer{
		Name: "crm", Transport: domain.TransportHTTP,
		URL: "https://tools.example.com/mcp", Enabled: true,
	}, domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	var detail map[string]any
	if err := pool.QueryRow(ctx, `
		select detail from admin_events
		where action = 'mcp_server.configured' and target = 'crm'
		order by at desc limit 1`).Scan(&detail); err != nil {
		t.Fatalf("read the trail: %v", err)
	}
	if detail["transport"] != "http" {
		t.Errorf("detail = %v, want the transport under its own name", detail)
	}
}
