package main

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/vault"
)

/*
From the reconciler finding nobody to discover dpg as to the engine asking the
worker's production view whether a run is refused for good: the wait recorded
in Postgres under the server's generation as read back, and asked again of the
store. The generation is a timestamp that round-trips through two tables; a
precision lost on either side would leave every run retrying for ever with
nothing failing.
*/
func TestAwaitingJourney_nobodyConnected_refusesARunUntilAPersonConnects(t *testing.T) {
	t.Parallel()
	pool := testdb.Isolated(t)
	ctx := context.WithoutCancel(t.Context())
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	health := admin.NewHealth(pool)
	integrations := admin.NewIntegrations(pool, settings.NewStore(pool, v)).ForgettingHealth(health)
	_, url := newIdentityServer(t)
	installation := domain.Scope{Company: "default", Area: "platform"}
	if _, err := integrations.WriteMCPServer(ctx, admin.MCPServerWrite{
		By: "usr_admin", Scope: installation,
		Server: domain.MCPServer{Name: "dpg", Transport: domain.TransportHTTP, URL: url, Enabled: true},
	}); err != nil {
		t.Fatalf("WriteMCPServer: %v", err)
	}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	r := newReconciler(catalog, integrations, health).withCredentialPolicy(everyonePersonal{})
	servers := (&workerParts{integrations: integrations, health: health}).deps(nil, nil).Servers

	r.reconcile(ctx)
	if waiting, err := servers.AwaitingPerson(ctx, "dpg"); err != nil || !waiting {
		t.Fatalf("with nobody connected AwaitingPerson = %v, %v, want waiting", waiting, err)
	}

	token := "ana"
	if err := integrations.PutMCPPersonalCredential(ctx, "usr_ana", installation, "dpg",
		domain.MCPCredentialPatch{Token: &token}); err != nil {
		t.Fatalf("PutMCPPersonalCredential: %v", err)
	}
	if waiting, err := servers.AwaitingPerson(ctx, "dpg"); err != nil || waiting {
		t.Errorf("once Ana connected AwaitingPerson = %v, %v, want no wait before any worker catches up", waiting, err)
	}
}
