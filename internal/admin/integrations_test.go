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
		`delete from settings where kind in ('mcp_server', 'model_provider')`); err != nil {
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
	err := i.PutMCPServer(context.Background(), "usr_ana", platform, domain.MCPServer{Name: "crm"})
	if err == nil {
		t.Fatal("PutMCPServer accepted a server with no command")
	}
}

func TestDeleteMCPServer_isRecorded(t *testing.T) {
	i := newIntegrations(t)
	pool := openPool(t)
	ctx := context.Background()

	if err := i.PutMCPServer(ctx, "usr_ana", platform,
		domain.MCPServer{Name: "crm", Command: "/usr/local/bin/crm-mcp", Enabled: true}); err != nil {
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
