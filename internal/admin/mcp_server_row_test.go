package admin_test

import (
	"context"
	"encoding/base64"
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
	// Even at the very revision the worker read: where it is stored decides.
	if _, err := w.pool.Exec(context.Background(), `
		update settings set updated_at = $1 where kind = 'mcp_server' and name = 'github'`, read.UpdatedAt); err != nil {
		t.Fatalf("align the revision: %v", err)
	}

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

var oldGrant = domain.MCPOAuthGrant{
	AccessToken: "old-access", RefreshToken: "old-refresh",
	TokenURL: "https://issuer.example/token", ClientID: "client", ExpiresAtUnix: 100,
}

func refreshedGrant() domain.MCPOAuthGrant {
	next := oldGrant
	next.AccessToken, next.ExpiresAtUnix = "new-access", 200
	return next
}

/*
A late refresh of a server's grant does not land on another of its name.

The server at acme/cx is moved to acme/finance holding the very same grant, so
the grant the worker started from still matches. The refresh names the row it
was read from, and the finance row keeps its grant.
*/
func TestRefreshMCPServerOAuth_ofAServerStoredElsewhereNow_leavesItAlone(t *testing.T) {
	w := withAreaServerHolding(t, domain.MCPCredentialPatch{OAuth: &oldGrant})
	ctx := context.Background()
	moveServer(t, w, cxScope, financeScope)

	err := w.integrations.RefreshMCPServerOAuth(ctx, admin.MCPOAuthRefresh{
		Server: "github", Scope: cxScope, Was: oldGrant, Next: refreshedGrant(),
	})
	if err == nil {
		t.Error("the refresh of the server at acme/cx was written")
	}
	row, err := w.store.Reveal(ctx, settings.ScopeArea, financeScope, settings.KindMCPServer, "github")
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	if got := domain.ReadMCPCredentials(row.Secret).OAuth; got == nil || got.AccessToken != "old-access" {
		t.Errorf("finance's grant = %+v, want it untouched", got)
	}
}

/*
A person's credential is for the server it was stored for (NT-019).

Stored while the server was at acme/cx, it is not handed to a worker connected
to a server of the name at acme/finance, and a late refresh from finance does
not rewrite it.
*/
func TestMCPPersonalCredential_isForTheServerRowItWasStoredFor(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	if err := w.integrations.PutMCPPersonalCredential(ctx, "usr_ana", platform, "github",
		domain.MCPCredentialPatch{OAuth: &oldGrant}); err != nil {
		t.Fatalf("PutMCPPersonalCredential: %v", err)
	}

	if _, found, err := w.integrations.MCPPersonalCredential(ctx, financeScope, "github", "usr_ana"); err != nil || found {
		t.Errorf("at finance: found = %v, %v; want no credential for another server", found, err)
	}
	if err := w.integrations.RefreshMCPServerOAuth(ctx, admin.MCPOAuthRefresh{
		Server: "github", Scope: financeScope, Principal: "usr_ana", Was: oldGrant, Next: refreshedGrant(),
	}); err == nil {
		t.Error("a refresh from another server rewrote the credential")
	}
	creds, found, err := w.integrations.MCPPersonalCredential(ctx, cxScope, "github", "usr_ana")
	if err != nil || !found || creds.OAuth == nil || creds.OAuth.AccessToken != "old-access" {
		t.Errorf("at cx: %+v, %v, %v; want the credential, untouched", creds.OAuth, found, err)
	}
}

// Refreshed, a person's credential stays the credential of its server row.
func TestRefreshMCPServerOAuth_aPersonalGrant_keepsItsServerRow(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	if err := w.integrations.PutMCPPersonalCredential(ctx, "usr_ana", platform, "github",
		domain.MCPCredentialPatch{OAuth: &oldGrant}); err != nil {
		t.Fatalf("PutMCPPersonalCredential: %v", err)
	}
	if err := w.integrations.RefreshMCPServerOAuth(ctx, admin.MCPOAuthRefresh{
		Server: "github", Scope: cxScope, Principal: "usr_ana", Was: oldGrant, Next: refreshedGrant(),
	}); err != nil {
		t.Fatalf("RefreshMCPServerOAuth: %v", err)
	}
	creds, found, err := w.integrations.MCPPersonalCredential(ctx, cxScope, "github", "usr_ana")
	if err != nil || !found || creds.OAuth == nil || creds.OAuth.AccessToken != "new-access" {
		t.Errorf("at cx: %+v, %v, %v; want the refreshed credential", creds.OAuth, found, err)
	}
}

// A person's credential stored by the release before this one named no
// server row: every server was the installation's then.
func TestMCPPersonalCredential_storedBeforeItNamedARow_isTheInstallationServers(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	name := base64.RawURLEncoding.EncodeToString([]byte("legacy")) + "." +
		base64.RawURLEncoding.EncodeToString([]byte("usr_ana"))
	if err := w.store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation, Kind: settings.KindMCPUserCredential, Name: name,
		Value:  []byte(`{"server":"legacy","principal":"usr_ana"}`),
		Secret: domain.MCPCredentials{Token: "legacy-token"}.Sealed(), Enabled: true, UpdatedBy: "usr_ana",
	}); err != nil {
		t.Fatalf("plant: %v", err)
	}
	if _, found, _ := w.integrations.MCPPersonalCredential(ctx, cxScope, "legacy", "usr_ana"); found {
		t.Error("an unnamed credential was handed for a server of acme/cx")
	}
	creds, found, err := w.integrations.MCPPersonalCredential(ctx, domain.Scope{}, "legacy", "usr_ana")
	if err != nil || !found || creds.Token != "legacy-token" {
		t.Errorf("at the installation: %+v, %v, %v; want the credential", creds, found, err)
	}
}

// Stored again for a server of the name stored elsewhere, a person's
// credential keeps nothing of the one they had for the first.
func TestPutMCPPersonalCredential_forAnotherServerRow_keepsNothingOfTheOld(t *testing.T) {
	w := withAreaServer(t)
	ctx := context.Background()
	if err := w.integrations.PutMCPPersonalCredential(ctx, "usr_ana", platform, "github",
		domain.MCPCredentialPatch{OAuth: &oldGrant}); err != nil {
		t.Fatalf("PutMCPPersonalCredential at cx: %v", err)
	}
	moveServer(t, w, cxScope, financeScope)
	if err := w.integrations.PutMCPPersonalCredential(ctx, "usr_ana", platform, "github",
		domain.MCPCredentialPatch{Token: ptr("finance-token")}); err != nil {
		t.Fatalf("PutMCPPersonalCredential at finance: %v", err)
	}
	creds, found, err := w.integrations.MCPPersonalCredential(ctx, financeScope, "github", "usr_ana")
	if err != nil || !found || creds.Token != "finance-token" || creds.OAuth != nil {
		t.Errorf("at finance: %+v, %v, %v; want only the token given for finance", creds, found, err)
	}
}
