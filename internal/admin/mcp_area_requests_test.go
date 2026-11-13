package admin_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
A door's request names the row the door authorized (NT-019).

The doors pass the exact row they authorized to RequestMCPProbeAt,
RequestMCPRediscoveryAt and MCPRediscoveryAt. The request keeps that row for
the worker, and the status of a row shows only its own request and receipts:
the administrator of a server stored at acme/finance is not shown what was
asked of, or answered for, a server of the name once stored at acme/cx.
*/

var cxRow = admin.MCPServerAt{Kind: settings.ScopeArea, Stored: cxScope, Audit: cxScope}

func TestMCPServerRequestsAt_nameTheRowTheDoorAuthorized(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	if _, err := w.pool.Exec(ctx, `delete from mcp_rediscoveries`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if err := w.integrations.RequestMCPProbeAt(ctx, "usr_ana", cxRow, "github"); err != nil {
		t.Fatalf("RequestMCPProbeAt: %v", err)
	}
	if _, err := w.integrations.RequestMCPRediscoveryAt(ctx, "usr_ana", cxRow, "github"); err != nil {
		t.Fatalf("RequestMCPRediscoveryAt: %v", err)
	}
	probes, err := w.integrations.ClaimMCPProbes(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimMCPProbes: %v", err)
	}
	if len(probes) != 1 || probes[0].Scope != cxScope {
		t.Errorf("probes = %+v, want one about the server at %s", probes, cxScope)
	}
	requests, err := w.integrations.MCPRediscoveries(ctx)
	if err != nil {
		t.Fatalf("MCPRediscoveries: %v", err)
	}
	if got := requests["github"]; got.Scope != cxScope {
		t.Errorf("rediscovery = %+v, want one about the server at %s", got, cxScope)
	}
}

func TestMCPRediscoveryAt_ofARowStoredSinceTheName_showsNothingOfTheFirst(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	if _, err := w.pool.Exec(ctx, `delete from mcp_rediscoveries; delete from mcp_rediscovery_receipts`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if _, err := w.integrations.RequestMCPRediscoveryAt(ctx, "usr_ana", cxRow, "github"); err != nil {
		t.Fatalf("RequestMCPRediscoveryAt: %v", err)
	}
	if err := w.integrations.RecordMCPRediscovery(ctx, domain.MCPRediscoveryReceipt{
		Server: "github", Worker: "w1", Generation: 1, Succeeded: true, Scope: cxScope,
	}); err != nil {
		t.Fatalf("RecordMCPRediscovery: %v", err)
	}
	if before, err := w.integrations.MCPRediscoveryAt(ctx, cxRow, "github"); err != nil || before.Generation != 1 || len(before.Receipts) != 1 {
		t.Fatalf("before the move: %+v, %v; want the request and receipt of the server at %s", before, err, cxScope)
	}
	moveServer(t, w, cxScope, financeScope)

	financeRow := admin.MCPServerAt{Kind: settings.ScopeArea, Stored: financeScope, Audit: financeScope}
	status, err := w.integrations.MCPRediscoveryAt(ctx, financeRow, "github")
	if err != nil {
		t.Fatalf("MCPRediscoveryAt: %v", err)
	}
	if status.Generation != 0 || status.RequestedBy != "" || len(status.Receipts) != 0 {
		t.Errorf("status = %+v, want nothing of the server stored at %s", status, cxScope)
	}
}
