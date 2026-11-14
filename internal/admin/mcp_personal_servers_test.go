package admin_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
The servers a person may hold a personal credential for, by the rule the write
already applies (NT-019): a remote server whose scope their tool:read reaches,
either way. An area's author reads tools there and cannot list the servers'
configuration, so without this they could not find the server their agent
needs a credential for. No address is answered: that is provider:write's.
*/

func TestMCPPersonalCredentialServers_offersAServerTheCallersScopeReaches(t *testing.T) {
	w := withAreaServer(t)
	for _, visible := range []domain.Scope{cxScope, {Company: "acme"}} {
		servers, err := w.integrations.MCPPersonalCredentialServers(context.Background(), []domain.Scope{visible})
		if err != nil {
			t.Fatalf("MCPPersonalCredentialServers(%s): %v", visible, err)
		}
		want := domain.MCPCredentialServer{Name: "github", Scope: cxScope, Shared: true}
		if len(servers) != 1 || servers[0] != want {
			t.Errorf("from %s: servers = %+v, want %+v", visible, servers, want)
		}
	}
}

func TestMCPPersonalCredentialServers_aSiblingAreasServer_isNotOffered(t *testing.T) {
	w := withAreaServer(t)
	servers, err := w.integrations.MCPPersonalCredentialServers(context.Background(),
		[]domain.Scope{{Company: "acme", Area: "ops"}})
	if err != nil {
		t.Fatalf("MCPPersonalCredentialServers: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("servers = %+v, want none of a sibling area's", servers)
	}
}

// A local server runs as the installation; nobody's personal credential is
// sent to it, and the write refuses one.
func TestMCPPersonalCredentialServers_aLocalServer_isNotOffered(t *testing.T) {
	w := withAreaServer(t)
	if err := w.store.Put(context.Background(), settings.Setting{
		ScopeKind: settings.ScopeInstallation, Kind: settings.KindMCPServer, Name: "reports",
		Value: []byte(`{"transport":"stdio","command":"/usr/bin/reports"}`), Enabled: true, UpdatedBy: "usr_ana",
	}); err != nil {
		t.Fatalf("store a local server: %v", err)
	}
	servers, err := w.integrations.MCPPersonalCredentialServers(context.Background(), []domain.Scope{cxScope})
	if err != nil {
		t.Fatalf("MCPPersonalCredentialServers: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "github" {
		t.Errorf("servers = %+v, want github alone", servers)
	}
}
