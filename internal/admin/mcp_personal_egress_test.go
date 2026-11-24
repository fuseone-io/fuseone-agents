package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

/*
A person's credential is held to the egress policy of the server it is for,
as the server's own credential is (NT-019).

The worker refreshes a personal grant at its token endpoint through the same
egress guard as the shared one (0.109.0). Stored without the check, a token
endpoint outside the policy is a credential that saves, then fails on the first
refresh, with nobody told at the moment they could have fixed it.
*/

const copilotOrigin = "https://api.githubcopilot.com"

func personalGrant(tokenURL string) domain.MCPOAuthGrant {
	return domain.MCPOAuthGrant{
		AccessToken: "personal-access", RefreshToken: "personal-refresh",
		TokenURL: tokenURL, ExpiresAtUnix: 4102444800,
	}
}

func putPersonalGrant(w areaServerWorld, grant domain.MCPOAuthGrant) error {
	return w.integrations.PutMCPPersonalCredential(context.Background(), "usr_ana", cxScope, "github",
		domain.MCPCredentialPatch{OAuth: &grant})
}

func personalGeneration(t *testing.T, w areaServerWorld) string {
	t.Helper()
	creds, found, err := w.integrations.MCPPersonalCredential(context.Background(), cxScope, "github", "usr_ana")
	if err != nil || !found || creds.OAuth == nil {
		t.Fatalf("MCPPersonalCredential = %+v, %v, %v; want the grant", creds.OAuth, found, err)
	}
	return creds.OAuth.Generation
}

func TestPutMCPPersonalCredential_aTokenEndpointOutsideTheServersPolicy_isRefused(t *testing.T) {
	w := withAreaServer(t)
	allowEgress(t, w.integrations, cxScope, copilotOrigin)

	err := putPersonalGrant(w, personalGrant("https://issuer.example.com/token?tenant=secret"))
	var refused *admin.EgressRefusal
	if !errors.As(err, &refused) || refused.Origin != "https://issuer.example.com:443" {
		t.Fatalf("err = %v, want an egress refusal naming https://issuer.example.com:443 alone", err)
	}
	if _, found, _ := w.integrations.MCPPersonalCredential(context.Background(), cxScope, "github", "usr_ana"); found {
		t.Error("the refused grant was stored")
	}

	allowEgress(t, w.integrations, cxScope, copilotOrigin, "https://issuer.example.com")
	if err := putPersonalGrant(w, personalGrant("https://issuer.example.com/token")); err != nil {
		t.Errorf("token endpoint listed: %v", err)
	}
}

// The policy is the area's when it has one, else its company's.
func TestPutMCPPersonalCredential_isHeldToTheCompanysPolicyUnlessTheAreaHasItsOwn(t *testing.T) {
	w := withAreaServer(t)
	allowEgress(t, w.integrations, domain.Scope{Company: "acme"}, copilotOrigin, "https://issuer.example.com")
	grant := personalGrant("https://issuer.example.com/token")
	if err := putPersonalGrant(w, grant); err != nil {
		t.Fatalf("under acme's policy: %v", err)
	}

	allowEgress(t, w.integrations, cxScope, copilotOrigin)
	if err := putPersonalGrant(w, grant); !errors.Is(err, admin.ErrEgressNotAllowed) {
		t.Errorf("under cx's own policy: err = %v, want ErrEgressNotAllowed", err)
	}
}

// The installation's servers are held to no policy, and neither are the
// credentials people keep for them.
func TestPutMCPPersonalCredential_forTheInstallationsServer_needsNoPolicy(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	if err := w.integrations.PutMCPServer(ctx, "usr_ana", platform, remoteServer("platform-jira"),
		domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
	grant := personalGrant("https://issuer.example.com/token")
	if err := w.integrations.PutMCPPersonalCredential(ctx, "usr_ana", cxScope, "platform-jira",
		domain.MCPCredentialPatch{OAuth: &grant}); err != nil {
		t.Errorf("PutMCPPersonalCredential for the installation's server: %v", err)
	}
}

// Read before the write's transaction, a policy narrowed meanwhile would let
// the door store what the platform had just refused.
func TestPutMCPPersonalCredential_waitsForAPolicyBeingWritten_andJudgesWhatItCommitted(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	allowEgress(t, w.integrations, cxScope, copilotOrigin, "https://issuer.example.com")

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := admin.LockMCPEgressPolicies(ctx, tx); err != nil {
		t.Fatalf("hold the policies: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		update settings set value = '{"company":"acme","area":"cx","origins":[]}'
		where kind = 'mcp_egress_policy' and name = 'acme/cx'`); err != nil {
		t.Fatalf("narrow the policy: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- putPersonalGrant(w, personalGrant("https://issuer.example.com/token")) }()
	if !waitForAdvisoryWait(t, w.pool) {
		t.Fatal("the personal write never waited on the policies' lock")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-done; !errors.Is(err, admin.ErrEgressNotAllowed) {
		t.Errorf("err = %v, want ErrEgressNotAllowed under the policy committed meanwhile", err)
	}
}

// A person sending the tokens they hold changes nothing; new tokens are a new
// grant, as on the server's own credential.
func TestPutMCPPersonalCredential_movesTheGrantsGenerationOnlyForNewTokens(t *testing.T) {
	w := withAreaServer(t)
	allowEgress(t, w.integrations, cxScope, copilotOrigin, "https://issuer.example.com")
	grant := personalGrant("https://issuer.example.com/token")
	if err := putPersonalGrant(w, grant); err != nil {
		t.Fatalf("first grant: %v", err)
	}
	first := personalGeneration(t, w)
	if first == "" {
		t.Fatal("the first grant has no generation")
	}

	if err := putPersonalGrant(w, grant); err != nil {
		t.Fatalf("same grant: %v", err)
	}
	if again := personalGeneration(t, w); again != first {
		t.Errorf("the same tokens moved the generation: %q, then %q", first, again)
	}

	grant.AccessToken = "personal-access-2"
	if err := putPersonalGrant(w, grant); err != nil {
		t.Fatalf("new tokens: %v", err)
	}
	if next := personalGeneration(t, w); next == first {
		t.Errorf("new tokens kept the generation %q", first)
	}
}
