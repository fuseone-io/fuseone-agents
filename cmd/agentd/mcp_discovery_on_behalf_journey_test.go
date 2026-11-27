package main

import (
	"context"
	"slices"
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
From a person connecting their account to the worker discovering as them, through
the real administration: the server with no shared credential, each person's
credential written as the console writes it, and the reconciler reading who
discovers, opening the session and writing the trail through the store
production hands it. A method the worker finds by assertion that drifted would
leave the server undiscovered with nothing failing.
*/
func TestDiscoveryJourney_aPersonConnecting_isWhoTheWorkerDiscoversAs(t *testing.T) {
	t.Parallel()
	pool := testdb.Isolated(t)
	ctx := context.WithoutCancel(t.Context())
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	integrations := admin.NewIntegrations(pool, settings.NewStore(pool, v))
	ids, url := newIdentityServer(t)
	installation := domain.Scope{Company: "default", Area: "platform"}
	if _, err := integrations.WriteMCPServer(ctx, admin.MCPServerWrite{
		By: "usr_admin", Scope: installation,
		Server: domain.MCPServer{Name: "dpg", Transport: domain.TransportHTTP, URL: url, Enabled: true},
	}); err != nil {
		t.Fatalf("WriteMCPServer: %v", err)
	}
	connect := func(who domain.UserID, token string) {
		t.Helper()
		if err := integrations.PutMCPPersonalCredential(ctx, who, installation, "dpg",
			domain.MCPCredentialPatch{Token: &token}); err != nil {
			t.Fatalf("PutMCPPersonalCredential(%s): %v", who, err)
		}
	}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	r := newReconciler(catalog, integrations, recordingHealth{}).withCredentialPolicy(everyonePersonal{})

	r.reconcile(ctx)
	if catalog.Connected("dpg") {
		t.Fatal("dpg connected with nobody to discover as")
	}

	connect("usr_ana", "ana")
	r.reconcile(ctx)
	connect("usr_bia", "bia")
	r.reconcile(ctx)

	if !catalog.Connected("dpg") {
		t.Fatal("dpg is not connected")
	}
	if got := ids.initializedWith(); !slices.Equal(got, []string{"Bearer ana", "Bearer bia"}) {
		t.Errorf("sessions initialized with %v, want Ana's discovery, then Bia's once she connected", got)
	}
	var recorded []string
	rows, err := pool.Query(ctx, `select detail->>'principal' from admin_events
		where action = 'mcp_server.discovered_on_behalf' and target = 'dpg' order by event_id`)
	if err != nil {
		t.Fatalf("read the trail: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var who string
		if err := rows.Scan(&who); err != nil {
			t.Fatalf("scan: %v", err)
		}
		recorded = append(recorded, who)
	}
	if !slices.Equal(recorded, []string{"usr_ana", "usr_bia"}) {
		t.Errorf("trail = %v, want Ana's discovery, then Bia's", recorded)
	}
}
