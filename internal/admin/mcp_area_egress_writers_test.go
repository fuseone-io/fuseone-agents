package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
An area's server is written only inside its scope's egress policy (NT-019).

The worker already holds the connection to the policy (0.109.0). The door
says so first: a server whose URL, OAuth token endpoint or GitHub App API base
is outside the list, or one whose scope the platform allows nothing, is
refused rather than stored to fail on every connection.
*/

func allowEgress(t *testing.T, i *admin.Integrations, at domain.Scope, origins ...string) {
	t.Helper()
	if origins == nil {
		origins = []string{}
	}
	if err := i.PutMCPEgressPolicy(context.Background(), "usr_ana", platform, admin.MCPEgressPolicy{
		Scope: at, Origins: origins,
	}); err != nil {
		t.Fatalf("PutMCPEgressPolicy %s: %v", at, err)
	}
	t.Cleanup(func() {
		_ = i.DeleteMCPEgressPolicy(context.Background(), "usr_ana", platform, at)
	})
}

func writeAtCX(i *admin.Integrations, name string, given domain.MCPCredentialPatch) error {
	_, err := i.WriteMCPServer(context.Background(), admin.MCPServerWrite{
		By: "usr_cx", Scope: cxScope, At: settings.ScopeArea, Stored: cxScope,
		Server: remoteServer(name), Given: given,
	})
	return err
}

func TestWriteMCPServer_anAreasServer_isRefusedOutsideItsEgressPolicy(t *testing.T) {
	i := newIntegrations(t)
	if err := writeAtCX(i, "cx-none", domain.MCPCredentialPatch{}); !errors.Is(err, admin.ErrEgressNotAllowed) {
		t.Errorf("with no policy: err = %v, want ErrEgressNotAllowed", err)
	}
	allowEgress(t, i, cxScope)
	if err := writeAtCX(i, "cx-deny", domain.MCPCredentialPatch{}); !errors.Is(err, admin.ErrEgressNotAllowed) {
		t.Errorf("with a deny-all: err = %v, want ErrEgressNotAllowed", err)
	}
	allowEgress(t, i, cxScope, "https://other.example.com")
	if err := writeAtCX(i, "cx-other", domain.MCPCredentialPatch{}); !errors.Is(err, admin.ErrEgressNotAllowed) {
		t.Errorf("with another origin: err = %v, want ErrEgressNotAllowed", err)
	}
}

func TestWriteMCPServer_anAreasServer_insideItsEgressPolicy_isStored(t *testing.T) {
	i := newIntegrations(t)
	allowEgress(t, i, domain.Scope{Company: "acme"}, "https://mcp.example.com")
	if err := writeAtCX(i, "cx-inherits", domain.MCPCredentialPatch{Token: ptr("t")}); err != nil {
		t.Errorf("under acme's policy: %v", err)
	}
}

// The token endpoint and the App's API base are requests of the server too.
func TestWriteMCPServer_anAreasServersTokenEndpoints_areHeldToThePolicy(t *testing.T) {
	i := newIntegrations(t)
	allowEgress(t, i, cxScope, "https://mcp.example.com")
	grant := domain.MCPOAuthGrant{
		AccessToken: "a", RefreshToken: "r", TokenURL: "https://issuer.example.com/token",
		ExpiresAtUnix: time.Now().Add(time.Hour).Unix(),
	}
	if err := writeAtCX(i, "cx-oauth", domain.MCPCredentialPatch{OAuth: &grant}); !errors.Is(err, admin.ErrEgressNotAllowed) {
		t.Errorf("token endpoint outside: err = %v, want ErrEgressNotAllowed", err)
	}
	allowEgress(t, i, cxScope, "https://mcp.example.com", "https://issuer.example.com")
	if err := writeAtCX(i, "cx-oauth", domain.MCPCredentialPatch{OAuth: &grant}); err != nil {
		t.Errorf("token endpoint listed: %v", err)
	}
}

// The installation's servers are not held to any policy.
func TestWriteMCPServer_theInstallationsServer_needsNoEgressPolicy(t *testing.T) {
	i := newIntegrations(t)
	if _, err := i.WriteMCPServer(context.Background(), admin.MCPServerWrite{
		By: "usr_ana", Scope: platform, Server: remoteServer("platform-jira"),
	}); err != nil {
		t.Errorf("WriteMCPServer at the installation: %v", err)
	}
}

// A GitHub App mints at its API base, api.github.com when none is named.
func TestWriteMCPServer_anAreasGitHubAppOutsideThePolicy_isRefused(t *testing.T) {
	i := newIntegrations(t)
	allowEgress(t, i, cxScope, "https://mcp.example.com")
	err := writeAtCX(i, "cx-app", domain.MCPCredentialPatch{GitHubApp: &domain.MCPGitHubApp{
		AppID: 1, InstallationID: 2, PrivateKey: "not-a-key",
	}})
	if !errors.Is(err, admin.ErrEgressNotAllowed) {
		t.Errorf("err = %v, want ErrEgressNotAllowed for the default API base", err)
	}
}

/*
A server write and a policy write take turns (NT-019).

Read before the server's transaction, or inside it without a lock, a policy
could be narrowed between the read and the commit: the door would store a
server the platform had just refused. A policy write holds one lock for its
whole transaction; a server write holds the same lock shared while it reads
the policy and until it commits.
*/
func TestWriteMCPServer_waitsForAPolicyBeingWritten_andJudgesWhatItCommitted(t *testing.T) {
	i := newIntegrations(t)
	pool := openPool(t)
	ctx := context.Background()
	allowEgress(t, i, cxScope, "https://mcp.example.com")

	tx, err := pool.Begin(ctx)
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
	go func() { done <- writeAtCX(i, "cx-race", domain.MCPCredentialPatch{}) }()
	if !waitForAdvisoryWait(t, pool) {
		t.Fatal("the server write never waited on the policies' lock")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-done; !errors.Is(err, admin.ErrEgressNotAllowed) {
		t.Errorf("err = %v, want ErrEgressNotAllowed under the policy committed meanwhile", err)
	}
}

func TestPutMCPEgressPolicy_waitsForAServerWriteThatReadThePolicy(t *testing.T) {
	i := newIntegrations(t)
	pool := openPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := admin.LockMCPEgressPoliciesShared(ctx, tx); err != nil {
		t.Fatalf("hold the policies shared: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- i.PutMCPEgressPolicy(ctx, "usr_ana", platform, admin.MCPEgressPolicy{Scope: cxScope, Origins: []string{}})
	}()
	if !waitForAdvisoryWait(t, pool) {
		t.Fatal("the policy write never waited for the server write holding the policies")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-done; err != nil {
		t.Errorf("PutMCPEgressPolicy after the server write: %v", err)
	}
	t.Cleanup(func() { _ = i.DeleteMCPEgressPolicy(context.Background(), "usr_ana", platform, cxScope) })
}

// Removing a policy widens nothing by surprise either: it waits too.
func TestDeleteMCPEgressPolicy_waitsForAServerWriteThatReadThePolicy(t *testing.T) {
	i := newIntegrations(t)
	pool := openPool(t)
	ctx := context.Background()
	allowEgress(t, i, cxScope)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := admin.LockMCPEgressPoliciesShared(ctx, tx); err != nil {
		t.Fatalf("hold the policies shared: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- i.DeleteMCPEgressPolicy(ctx, "usr_ana", platform, cxScope) }()
	if !waitForAdvisoryWait(t, pool) {
		t.Fatal("the policy removal never waited for the server write holding the policies")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-done; err != nil {
		t.Errorf("DeleteMCPEgressPolicy after the server write: %v", err)
	}
}
