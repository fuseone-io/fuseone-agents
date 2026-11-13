package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

func putRediscoverable(t *testing.T, i *admin.Integrations, name string, enabled bool) {
	t.Helper()
	pool := openPool(t)
	clean := func() error {
		_, err := pool.Exec(context.Background(),
			`truncate mcp_rediscovery_receipts, mcp_rediscoveries;
			 delete from settings where kind = 'mcp_server' and name like 'rediscovery-%'`)
		return err
	}
	if err := clean(); err != nil {
		t.Fatalf("clean: %v", err)
	}
	// Removed after, not only before: the curator's tests run in parallel once
	// these finish, and a server left configured here turns their unmanaged
	// publications into judged ones.
	t.Cleanup(func() {
		if err := clean(); err != nil {
			t.Errorf("clean up: %v", err)
		}
	})
	if err := i.PutMCPServer(context.Background(), "usr_ana", platform,
		domain.MCPServer{
			Name: name, Command: "/usr/local/bin/crm-mcp", Enabled: enabled,
			AcceptsLocalExecution: true,
		}, domain.MCPCredentialPatch{}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}
}

// Asking every worker to look again is not a change to the server: its revision
// and its updated_at stay, so nothing reconnects and no client reads drift.
func TestRequestMCPRediscovery_leavesTheConfigurationAlone(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	putRediscoverable(t, i, "rediscovery-crm", true)
	before, err := i.MCPServerNamed(ctx, "rediscovery-crm")
	if err != nil {
		t.Fatalf("MCPServerNamed: %v", err)
	}

	if _, err := i.RequestMCPRediscovery(ctx, "usr_bia", platform, "rediscovery-crm"); err != nil {
		t.Fatalf("RequestMCPRediscovery: %v", err)
	}

	after, err := i.MCPServerNamed(ctx, "rediscovery-crm")
	if err != nil {
		t.Fatalf("MCPServerNamed: %v", err)
	}
	if after.Revision != before.Revision || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("server moved from %s/%v to %s/%v, want it untouched",
			before.Revision, before.UpdatedAt, after.Revision, after.UpdatedAt)
	}
}

// Each request is a new generation every worker owes, and it is on the trail
// under the person who asked.
func TestRequestMCPRediscovery_eachRequestIsTheNextGeneration_onTheTrail(t *testing.T) {
	i := newIntegrations(t)
	pool := openPool(t)
	ctx := context.Background()
	putRediscoverable(t, i, "rediscovery-crm", true)

	first, err := i.RequestMCPRediscovery(ctx, "usr_bia", platform, "rediscovery-crm")
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	second, err := i.RequestMCPRediscovery(ctx, "usr_bia", platform, "rediscovery-crm")
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	if second != first+1 {
		t.Fatalf("generations = %d then %d, want consecutive", first, second)
	}

	wanted, err := i.MCPRediscoveries(ctx)
	if err != nil {
		t.Fatalf("MCPRediscoveries: %v", err)
	}
	if wanted["rediscovery-crm"].Generation != second {
		t.Errorf("a worker reads generation %d, want %d", wanted["rediscovery-crm"].Generation, second)
	}
	var action, by string
	if err := pool.QueryRow(ctx,
		`select action, principal_id from admin_events where target = 'rediscovery-crm' order by event_id desc limit 1`,
	).Scan(&action, &by); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if action != "mcp_server.rediscovery_requested" || by != "usr_bia" {
		t.Errorf("event = %s by %s, want the rediscovery request by usr_bia", action, by)
	}
}

func TestRequestMCPRediscovery_refusesDisabledAndUnknownServers(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	putRediscoverable(t, i, "rediscovery-crm", false)

	if _, err := i.RequestMCPRediscovery(ctx, "usr_bia", platform, "rediscovery-crm"); !errors.Is(err, admin.ErrMCPServerDisabled) {
		t.Errorf("disabled = %v, want %v", err, admin.ErrMCPServerDisabled)
	}
	if _, err := i.RequestMCPRediscovery(ctx, "usr_bia", platform, "rediscovery-billing"); !errors.Is(err, admin.ErrMCPServerUnknown) {
		t.Errorf("unknown = %v, want %v", err, admin.ErrMCPServerUnknown)
	}
}

// Each worker's receipt is kept apart, and a late receipt for an older
// generation never hides that the worker already answered a newer one.
func TestRecordMCPRediscovery_keepsEachWorkersNewestReceipt(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	putRediscoverable(t, i, "rediscovery-crm", true)
	generation, err := i.RequestMCPRediscovery(ctx, "usr_bia", platform, "rediscovery-crm")
	if err != nil {
		t.Fatalf("RequestMCPRediscovery: %v", err)
	}

	for _, r := range []domain.MCPRediscoveryReceipt{
		{Server: "rediscovery-crm", Worker: "worker-a", Generation: generation, Succeeded: true},
		{Server: "rediscovery-crm", Worker: "worker-b", Generation: generation, Succeeded: false},
		{Server: "rediscovery-crm", Worker: "worker-a", Generation: generation - 1, Succeeded: false},
	} {
		if err := i.RecordMCPRediscovery(ctx, r); err != nil {
			t.Fatalf("RecordMCPRediscovery(%+v): %v", r, err)
		}
	}

	status, err := i.MCPRediscovery(ctx, "rediscovery-crm")
	if err != nil {
		t.Fatalf("MCPRediscovery: %v", err)
	}
	if status.Generation != generation || status.RequestedBy != "usr_bia" || len(status.Receipts) != 2 {
		t.Fatalf("status = %+v, want generation %d by usr_bia with two receipts", status, generation)
	}
	got := map[string]domain.MCPRediscoveryReceipt{}
	for _, r := range status.Receipts {
		got[r.Worker] = r
	}
	if a := got["worker-a"]; a.Generation != generation || !a.Succeeded {
		t.Errorf("worker-a = %+v, want its newest receipt, a success at %d", a, generation)
	}
	if b := got["worker-b"]; b.Generation != generation || b.Succeeded {
		t.Errorf("worker-b = %+v, want its failure at %d", b, generation)
	}
}

// A server nobody asked about reads as generation zero, with no receipts.
func TestMCPRediscovery_aServerNeverAsked_isGenerationZero(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	putRediscoverable(t, i, "rediscovery-crm", true)

	status, err := i.MCPRediscovery(ctx, "rediscovery-crm")
	if err != nil {
		t.Fatalf("MCPRediscovery: %v", err)
	}
	if status.Generation != 0 || len(status.Receipts) != 0 {
		t.Errorf("status = %+v, want generation zero and no receipts", status)
	}
}

// Reading the answers about a server that is not configured is a server not
// found, not an empty status that reads as "nobody asked".
func TestMCPRediscovery_anUnknownServer_isNotFound(t *testing.T) {
	i := newIntegrations(t)
	putRediscoverable(t, i, "rediscovery-crm", true)

	if _, err := i.MCPRediscovery(context.Background(), "rediscovery-billing"); !errors.Is(err, admin.ErrMCPServerUnknown) {
		t.Errorf("err = %v, want %v", err, admin.ErrMCPServerUnknown)
	}
}
