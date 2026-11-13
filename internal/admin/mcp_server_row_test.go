package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
A worker opens the credential of the row it connected, and no other (NT-019).

The worker reads the servers, then opens each one's credential. A server
removed from acme/cx and stored again under the same name in acme/finance
between the two is another server: its token must never travel with the URL
of the one the worker read. Nor may a later revision of the same row, whose
credential may belong to a URL the worker did not read.
*/

func snapshotOf(t *testing.T, w areaServerWorld) domain.MCPServer {
	t.Helper()
	servers, err := w.integrations.MCPServers(context.Background())
	if err != nil || len(servers) != 1 {
		t.Fatalf("MCPServers = %v, %v; want the one server", servers, err)
	}
	return servers[0]
}

func TestMCPServerCredentials_ofTheRowAsRead_areOpened(t *testing.T) {
	w := withAreaServer(t)
	creds, err := w.integrations.MCPServerCredentials(context.Background(), snapshotOf(t, w))
	if err != nil {
		t.Fatalf("MCPServerCredentials: %v", err)
	}
	if creds.Token != "ghp_area_secret" {
		t.Errorf("token = %q, want the area's", creds.Token)
	}
}

func TestMCPServerCredentials_ofANameStoredElsewhereNow_areNotOpened(t *testing.T) {
	w := withAreaServer(t)
	read := snapshotOf(t, w)
	moveServer(t, w, cxScope, financeScope)
	restoreToken(t, w, financeScope, "finance_secret")

	creds, err := w.integrations.MCPServerCredentials(context.Background(), read)
	if !errors.Is(err, admin.ErrMCPServerChanged) {
		t.Errorf("err = %v (token %q), want ErrMCPServerChanged", err, creds.Token)
	}
}

func TestMCPServerCredentials_ofARowRevisedSinceItWasRead_areNotOpened(t *testing.T) {
	w := withAreaServer(t)
	read := snapshotOf(t, w)
	restoreToken(t, w, cxScope, "rotated_secret")

	creds, err := w.integrations.MCPServerCredentials(context.Background(), read)
	if !errors.Is(err, admin.ErrMCPServerChanged) {
		t.Errorf("err = %v (token %q), want ErrMCPServerChanged", err, creds.Token)
	}
}

// restoreToken writes the area's row again with another token, as an
// administrator rotating it would.
func restoreToken(t *testing.T, w areaServerWorld, at domain.Scope, token string) {
	t.Helper()
	ctx := context.Background()
	row, err := w.store.Reveal(ctx, settings.ScopeArea, at, settings.KindMCPServer, "github")
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	if err := w.store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeArea, Scope: at, Kind: settings.KindMCPServer, Name: "github",
		Value: row.Value, Secret: domain.MCPCredentials{Token: token}.Sealed(),
		Enabled: true, UpdatedBy: "usr_bia",
	}); err != nil {
		t.Fatalf("store: %v", err)
	}
}
