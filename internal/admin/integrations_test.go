package admin_test

import (
	"context"
	"errors"
	"strings"
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
		`delete from settings where kind in ('mcp_server', 'model_provider', 'model_price')`); err != nil {
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
	err := i.PutMCPServer(context.Background(), "usr_ana", platform, domain.MCPServer{Name: "crm"}, "")
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
		}, ""); err != nil {
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
	}, "")
	if !errors.Is(err, admin.ErrNoURL) {
		t.Fatalf("PutMCPServer = %v, want a refusal naming the missing address", err)
	}
}

func TestPutMCPServer_local_stillNeedsACommand(t *testing.T) {
	i := newIntegrations(t)

	err := i.PutMCPServer(context.Background(), "usr_ana", platform, domain.MCPServer{
		Name: "crm", Transport: domain.TransportStdio, Enabled: true,
	}, "")
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
	}, "ghp_secret"); err != nil {
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
	token, err := i.MCPToken(ctx, "github")
	if err != nil {
		t.Fatalf("MCPToken: %v", err)
	}
	if token != "ghp_secret" {
		t.Errorf("token = %q", token)
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
	}, ""); err != nil {
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
		}, "")
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
	}, ""); err != nil {
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
		}, "tok"); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
}
