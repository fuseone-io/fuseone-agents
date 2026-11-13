package admin_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
A probe and a rediscovery name the row they were asked of (NT-019).

A server's name is unique, but not forever: removed from acme/cx and stored
again in acme/finance, it is another server. A request asked of the first
keeps where it was stored, so a worker holding the second does not answer it,
and the second's administrator is not shown the first's request or receipts.
*/

var financeScope = domain.Scope{Company: "acme", Area: "finance"}

func TestMCPServerRequests_nameTheRowTheyWereAskedOf(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	if _, err := w.pool.Exec(ctx, `delete from mcp_rediscoveries`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if err := w.integrations.RequestMCPProbe(ctx, "usr_ana", platform, "github"); err != nil {
		t.Fatalf("RequestMCPProbe: %v", err)
	}
	if _, err := w.integrations.RequestMCPRediscovery(ctx, "usr_ana", platform, "github"); err != nil {
		t.Fatalf("RequestMCPRediscovery: %v", err)
	}

	probes, err := w.integrations.ClaimMCPProbes(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimMCPProbes: %v", err)
	}
	if len(probes) != 1 || probes[0].Name != "github" || probes[0].Scope != cxScope {
		t.Errorf("probes = %+v, want one about the server at %s", probes, cxScope)
	}
	requests, err := w.integrations.MCPRediscoveries(ctx)
	if err != nil {
		t.Fatalf("MCPRediscoveries: %v", err)
	}
	if got := requests["github"]; got.Generation != 1 || got.Scope != cxScope {
		t.Errorf("rediscovery = %+v, want generation 1 about the server at %s", got, cxScope)
	}
}

// A probe stored by the release before this one named no row: every server
// was the installation's then.
func TestClaimMCPProbes_aRequestThatNamesNoRow_isTheInstallations(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	if err := w.store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation, Kind: "mcp_probe", Name: "legacy",
		Value: []byte(`{}`), Enabled: true, UpdatedBy: "usr_ana",
	}); err != nil {
		t.Fatalf("plant: %v", err)
	}
	probes, err := w.integrations.ClaimMCPProbes(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimMCPProbes: %v", err)
	}
	installation := domain.Scope{Company: domain.Installation}
	if len(probes) != 1 || probes[0].Scope != installation {
		t.Errorf("probes = %+v, want the installation's", probes)
	}
}

func TestMCPRediscovery_ofANameStoredElsewhereNow_showsNothingOfTheFirst(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	if _, err := w.pool.Exec(ctx, `delete from mcp_rediscoveries; delete from mcp_rediscovery_receipts`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if _, err := w.integrations.RequestMCPRediscovery(ctx, "usr_ana", platform, "github"); err != nil {
		t.Fatalf("RequestMCPRediscovery: %v", err)
	}
	if err := w.integrations.RecordMCPRediscovery(ctx, domain.MCPRediscoveryReceipt{
		Server: "github", Worker: "w1", Generation: 1, Succeeded: true, Scope: cxScope,
	}); err != nil {
		t.Fatalf("RecordMCPRediscovery: %v", err)
	}
	if before, err := w.integrations.MCPRediscovery(ctx, "github"); err != nil || len(before.Receipts) != 1 {
		t.Fatalf("before the move: %+v, %v; want the receipt of the server at %s", before, err, cxScope)
	}
	moveServer(t, w, cxScope, financeScope)

	status, err := w.integrations.MCPRediscovery(ctx, "github")
	if err != nil {
		t.Fatalf("MCPRediscovery: %v", err)
	}
	if status.Generation != 0 || status.RequestedBy != "" || len(status.Receipts) != 0 {
		t.Errorf("status = %+v, want nothing of the server stored at %s", status, cxScope)
	}
}

// moveServer removes the server from one area and stores it again in another,
// as the writers will let two administrators do.
func moveServer(t *testing.T, w areaServerWorld, from, to domain.Scope) {
	t.Helper()
	ctx := context.Background()
	row, err := w.store.Reveal(ctx, settings.ScopeArea, from, settings.KindMCPServer, "github")
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	if err := w.store.Delete(ctx, settings.ScopeArea, from, settings.KindMCPServer, "github"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := w.store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeArea, Scope: to, Kind: settings.KindMCPServer, Name: "github",
		Value: row.Value, Secret: row.Secret, Enabled: true, UpdatedBy: "usr_bia",
	}); err != nil {
		t.Fatalf("store: %v", err)
	}
}
