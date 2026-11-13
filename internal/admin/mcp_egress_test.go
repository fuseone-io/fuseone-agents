package admin_test

import (
	"context"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
The platform keeps where servers outside the installation may send requests
(NT-019, decided 2026-11-13).

A policy names a company or an area and a list of exact origins. A server of an
area is held to its area's policy, or else to its company's, never to both
together; with neither, it reaches nothing. The installation's servers carry
no policy and keep the reach they always had.
*/

var acmeScope = domain.Scope{Company: "acme"}

func putEgress(t *testing.T, i *admin.Integrations, at domain.Scope, origins ...string) {
	t.Helper()
	if err := i.PutMCPEgressPolicy(context.Background(), "usr_ana", platform, admin.MCPEgressPolicy{
		Scope: at, Origins: origins,
	}); err != nil {
		t.Fatalf("PutMCPEgressPolicy %s: %v", at, err)
	}
}

func cleanEgress(t *testing.T, w areaServerWorld) {
	t.Helper()
	clean := func() {
		if _, err := w.pool.Exec(context.Background(), `delete from settings where kind = 'mcp_egress_policy'`); err != nil {
			t.Fatalf("clean: %v", err)
		}
	}
	clean()
	t.Cleanup(clean)
}

func TestPutMCPEgressPolicy_storesExactOriginsOnce(t *testing.T) {
	w := withAreaServer(t)
	cleanEgress(t, w)
	putEgress(t, w.integrations, cxScope, "https://Jira.example.com/", "https://jira.example.com:443", "http://10.24.0.17:8080")

	policies, err := w.integrations.MCPEgressPolicies(context.Background())
	if err != nil {
		t.Fatalf("MCPEgressPolicies: %v", err)
	}
	want := []string{"http://10.24.0.17:8080", "https://jira.example.com:443"}
	if len(policies) != 1 || policies[0].Scope != cxScope || !slices.Equal(policies[0].Origins, want) {
		t.Errorf("policies = %+v, want one at %s with %v", policies, cxScope, want)
	}
}

func TestPutMCPEgressPolicy_refusesWhatCannotBeAPolicy(t *testing.T) {
	w := withAreaServer(t)
	cleanEgress(t, w)
	for name, p := range map[string]admin.MCPEgressPolicy{
		"the installation":     {Scope: domain.Scope{Company: domain.Installation}, Origins: []string{"https://jira.example.com"}},
		"no scope":             {Origins: []string{"https://jira.example.com"}},
		"an invalid company":   {Scope: domain.Scope{Company: "Acme"}, Origins: []string{"https://jira.example.com"}},
		"an area not folded":   {Scope: domain.Scope{Company: "acme", Area: "CX"}, Origins: []string{"https://jira.example.com"}},
		"no origin":            {Scope: cxScope},
		"a path":               {Scope: cxScope, Origins: []string{"https://jira.example.com/mcp"}},
		"a wildcard":           {Scope: cxScope, Origins: []string{"https://*.example.com"}},
		"cloud metadata":       {Scope: cxScope, Origins: []string{"http://169.254.169.254"}},
		"a link-local address": {Scope: cxScope, Origins: []string{"http://[fe80::1]:8080"}},
	} {
		if err := w.integrations.PutMCPEgressPolicy(context.Background(), "usr_ana", platform, p); err == nil {
			t.Errorf("%s: stored, want refused", name)
		}
	}
	if policies, _ := w.integrations.MCPEgressPolicies(context.Background()); len(policies) != 0 {
		t.Errorf("policies = %+v, want none stored", policies)
	}
}

func TestMCPServers_anAreasServer_isHeldToItsAreasPolicyElseItsCompanys(t *testing.T) {
	w := withAreaServer(t)
	cleanEgress(t, w)
	ctx := context.Background()
	egressOf := func() *domain.MCPEgress {
		t.Helper()
		servers, err := w.integrations.MCPServers(ctx)
		if err != nil || len(servers) != 1 {
			t.Fatalf("MCPServers = %v, %v", servers, err)
		}
		return servers[0].Egress
	}

	if got := egressOf(); got == nil || len(got.Origins) != 0 {
		t.Errorf("with no policy: egress = %+v, want one that allows nothing", got)
	}
	putEgress(t, w.integrations, acmeScope, "https://company.example.com")
	if got := egressOf(); got == nil || !slices.Equal(got.Origins, []string{"https://company.example.com:443"}) {
		t.Errorf("with acme's policy: egress = %+v, want acme's", got)
	}
	putEgress(t, w.integrations, cxScope, "https://area.example.com")
	if got := egressOf(); got == nil || !slices.Equal(got.Origins, []string{"https://area.example.com:443"}) {
		t.Errorf("with cx's and acme's: egress = %+v, want cx's alone", got)
	}
	if err := w.integrations.DeleteMCPEgressPolicy(ctx, "usr_ana", platform, cxScope); err != nil {
		t.Fatalf("DeleteMCPEgressPolicy: %v", err)
	}
	if got := egressOf(); got == nil || !slices.Equal(got.Origins, []string{"https://company.example.com:443"}) {
		t.Errorf("after cx's is removed: egress = %+v, want acme's again", got)
	}
}

func TestMCPServers_theInstallationsServer_carriesNoPolicy(t *testing.T) {
	w := withAreaServer(t)
	cleanEgress(t, w)
	moveToInstallation(t, w)
	putEgress(t, w.integrations, acmeScope, "https://company.example.com")
	servers, err := w.integrations.MCPServers(context.Background())
	if err != nil || len(servers) != 1 {
		t.Fatalf("MCPServers = %v, %v", servers, err)
	}
	if servers[0].Egress != nil {
		t.Errorf("egress = %+v, want none for the installation's server", servers[0].Egress)
	}
}

// moveToInstallation stores the area's "github" at the installation instead.
func moveToInstallation(t *testing.T, w areaServerWorld) {
	t.Helper()
	ctx := context.Background()
	row, err := w.store.Reveal(ctx, settings.ScopeArea, cxScope, settings.KindMCPServer, "github")
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	if err := w.store.Delete(ctx, settings.ScopeArea, cxScope, settings.KindMCPServer, "github"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := w.store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation, Kind: settings.KindMCPServer, Name: "github",
		Value: row.Value, Secret: row.Secret, Enabled: true, UpdatedBy: "usr_ana",
	}); err != nil {
		t.Fatalf("store: %v", err)
	}
}
