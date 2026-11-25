package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
A remote server's browser sign-in configuration (NT-020 plan, delivery 3a).

What is public — the client, the scopes, the endpoints — is the server row's
own value: the client ID travels in the address the browser opens. The client
secret is sealed in a row of its own beside it, at the same scope and name,
written in the same transaction, removed with the server and sealed again when
the server moves. A binary from before this ignores both.
*/

const signin = "signin"

func signinServer() domain.MCPServer {
	return domain.MCPServer{Name: signin, Transport: domain.TransportHTTP,
		URL: "https://mcp.example.com/signin", Enabled: true}
}

func withSigninWorld(t *testing.T) moveWorld {
	t.Helper()
	w := withMovableServer(t, true)
	t.Cleanup(func() {
		_, _ = w.pool.Exec(context.Background(), `delete from settings where kind = 'mcp_oauth_login'`)
	})
	return w
}

func writeLogin(w moveWorld, login *domain.MCPOAuthLoginPatch) (domain.Revision, error) {
	return w.integrations.WriteMCPServer(context.Background(), admin.MCPServerWrite{
		By: "usr_ana", Scope: platform, Server: signinServer(), Login: login,
	})
}

func loginSecretAt(w moveWorld, kind settings.ScopeKind, at domain.Scope) (string, error) {
	row, err := w.store.Reveal(context.Background(), kind, at, settings.KindMCPOAuthLogin, signin)
	return row.Secret, err
}

func TestWriteMCPServer_aSignInConfiguration_isReadOpenly_andItsSecretSealedApart(t *testing.T) {
	w := withSigninWorld(t)
	if _, err := writeLogin(w, &domain.MCPOAuthLoginPatch{
		ClientID: "agents", Scopes: []string{"openid"}, ClientSecret: ptr("s3cret"),
	}); err != nil {
		t.Fatalf("WriteMCPServer: %v", err)
	}

	got, err := w.integrations.MCPServerNamed(context.Background(), signin)
	if err != nil {
		t.Fatalf("MCPServerNamed: %v", err)
	}
	want := &domain.MCPOAuthLogin{ClientID: "agents", Scopes: []string{"openid"},
		TokenAuthMethod: domain.TokenAuthClientSecretBasic, HasClientSecret: true}
	if !got.OAuthLogin.Equal(want) {
		t.Errorf("OAuthLogin = %+v, want %+v", got.OAuthLogin, want)
	}
	row, err := w.store.Reveal(context.Background(), settings.ScopeInstallation, domain.Scope{}, settings.KindMCPServer, signin)
	if err != nil || strings.Contains(string(row.Value), "s3cret") || strings.Contains(row.Secret, "s3cret") {
		t.Errorf("server row = %s / %q, %v; the client secret is in neither", row.Value, row.Secret, err)
	}
	if secret, err := loginSecretAt(w, settings.ScopeInstallation, domain.Scope{}); err != nil || secret != "s3cret" {
		t.Errorf("sealed secret = %q, %v; want s3cret", secret, err)
	}
}

// The server's revision covers only its own row: a secret changed alone must
// still move it, or the write would be skipped as a change to nothing.
func TestWriteMCPServer_aNewClientSecretAlone_movesTheRevision(t *testing.T) {
	w := withSigninWorld(t)
	first, err := writeLogin(w, &domain.MCPOAuthLoginPatch{ClientID: "agents", ClientSecret: ptr("s1")})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	kept, err := writeLogin(w, &domain.MCPOAuthLoginPatch{ClientID: "agents"})
	if err != nil || kept != first {
		t.Errorf("secret omitted: revision %s, %v; want %s, unchanged", kept, err, first)
	}
	rotated, err := writeLogin(w, &domain.MCPOAuthLoginPatch{ClientID: "agents", ClientSecret: ptr("s2")})
	if err != nil || rotated == first {
		t.Fatalf("secret rotated: revision %s, %v; want one other than %s", rotated, err, first)
	}
	if secret, _ := loginSecretAt(w, settings.ScopeInstallation, domain.Scope{}); secret != "s2" {
		t.Errorf("sealed secret = %q, want s2", secret)
	}
}

func TestWriteMCPServer_aSignInConfiguration_isKeptWhenOmitted_andRemovedWithItsSecret(t *testing.T) {
	w := withSigninWorld(t)
	if _, err := writeLogin(w, &domain.MCPOAuthLoginPatch{ClientID: "agents", ClientSecret: ptr("s3cret")}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if _, err := writeLogin(w, nil); err != nil {
		t.Fatalf("an edit that omits it: %v", err)
	}
	if got, _ := w.integrations.MCPServerNamed(context.Background(), signin); got.OAuthLogin == nil {
		t.Fatal("an edit that omitted the configuration removed it")
	}

	if _, err := writeLogin(w, &domain.MCPOAuthLoginPatch{Remove: true}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got, _ := w.integrations.MCPServerNamed(context.Background(), signin); got.OAuthLogin != nil {
		t.Errorf("OAuthLogin = %+v after removal, want none", got.OAuthLogin)
	}
	if _, err := loginSecretAt(w, settings.ScopeInstallation, domain.Scope{}); !errors.Is(err, settings.ErrNotFound) {
		t.Errorf("sealed secret after removal: err = %v, want ErrNotFound", err)
	}
}

func TestWriteMCPServer_aSignInConfiguration_isRefusedWhereNoSignInCouldUseIt(t *testing.T) {
	w := withSigninWorld(t)
	login := &domain.MCPOAuthLoginPatch{ClientID: "agents"}
	local := domain.MCPServer{Name: "local-signin", Transport: domain.TransportStdio, Command: "sh",
		AcceptsLocalExecution: true, Enabled: true}
	for name, write := range map[string]admin.MCPServerWrite{
		"a local server": {Server: local, Login: login},
		"a GitHub App":   {Server: signinServer(), Login: login, Given: domain.MCPCredentialPatch{GitHubApp: &domain.MCPGitHubApp{AppID: 1, InstallationID: 2, PrivateKey: appKey(t)}}},
		"a metadata endpoint": {Server: signinServer(), Login: &domain.MCPOAuthLoginPatch{
			ClientID: "agents", TokenURL: "http://169.254.169.254/token"}},
	} {
		write.By, write.Scope = "usr_ana", platform
		if _, err := w.integrations.WriteMCPServer(context.Background(), write); !errors.Is(err, domain.ErrMCPOAuthLoginInvalid) {
			t.Errorf("%s: err = %v, want ErrMCPOAuthLoginInvalid", name, err)
		}
	}
}

// The endpoints are requests of the server's too, held to its scope's policy.
func TestWriteMCPServer_anAreasSignInEndpointOutsideItsPolicy_isRefused(t *testing.T) {
	w := withSigninWorld(t)
	_, err := w.integrations.WriteMCPServer(context.Background(), admin.MCPServerWrite{
		By: "usr_cx", Scope: cxScope, At: settings.ScopeArea, Stored: cxScope,
		Server: signinServer(),
		Login:  &domain.MCPOAuthLoginPatch{ClientID: "agents", AuthorizeURL: "https://issuer.example.org/authorize"},
	})
	var refused *admin.EgressRefusal
	if !errors.As(err, &refused) || refused.Origin != "https://issuer.example.org:443" {
		t.Errorf("err = %v, want an egress refusal naming https://issuer.example.org:443", err)
	}
}

func TestRemoveMCPServer_takesItsSignInSecretWithIt(t *testing.T) {
	w := withSigninWorld(t)
	if _, err := writeLogin(w, &domain.MCPOAuthLoginPatch{ClientID: "agents", ClientSecret: ptr("s3cret")}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := w.integrations.RemoveMCPServer(context.Background(), admin.Removal{
		By: "usr_ana", Scope: platform, Name: signin,
	}); err != nil {
		t.Fatalf("RemoveMCPServer: %v", err)
	}
	if _, err := loginSecretAt(w, settings.ScopeInstallation, domain.Scope{}); !errors.Is(err, settings.ErrNotFound) {
		t.Errorf("sealed secret after the server went: err = %v, want ErrNotFound", err)
	}
}

// Sealed under its scope, the secret is sealed again at the area it moves to:
// copied as it is, it would not open there.
func TestMoveMCPServer_sealsItsSignInSecretAgainAtTheArea(t *testing.T) {
	w := withSigninWorld(t)
	rev, err := w.integrations.WriteMCPServer(context.Background(), admin.MCPServerWrite{
		By: "usr_ana", Scope: platform,
		Server: domain.MCPServer{Name: movable, Transport: domain.TransportHTTP, URL: "https://mcp.example.com/mcp", Enabled: true},
		Login:  &domain.MCPOAuthLoginPatch{ClientID: "agents", ClientSecret: ptr("s3cret")},
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	w.revision = rev
	if _, err := w.move(t, admin.MCPServerMove{}); err != nil {
		t.Fatalf("move: %v", err)
	}
	row, err := w.store.Reveal(context.Background(), settings.ScopeArea, cxScope, settings.KindMCPOAuthLogin, movable)
	if err != nil || row.Secret != "s3cret" {
		t.Errorf("secret at the area = %q, %v; want s3cret, opened there", row.Secret, err)
	}
	if _, err := w.store.Reveal(context.Background(), settings.ScopeInstallation, domain.Scope{},
		settings.KindMCPOAuthLogin, movable); !errors.Is(err, settings.ErrNotFound) {
		t.Errorf("secret left at the installation: err = %v, want ErrNotFound", err)
	}
}

// Moved to an area, the sign-in's endpoints are held to that area's policy as
// the server's address is: refused before anything moves.
func TestMoveMCPServer_aSignInEndpointOutsideTheTargetsPolicy_isRefused(t *testing.T) {
	w := withSigninWorld(t)
	rev, err := w.integrations.WriteMCPServer(context.Background(), admin.MCPServerWrite{
		By: "usr_ana", Scope: platform,
		Server: domain.MCPServer{Name: movable, Transport: domain.TransportHTTP, URL: "https://mcp.example.com/mcp", Enabled: true},
		Login:  &domain.MCPOAuthLoginPatch{ClientID: "agents", TokenURL: "https://issuer.example.org/token"},
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	w.revision = rev
	var refused *admin.EgressRefusal
	if _, err := w.move(t, admin.MCPServerMove{}); !errors.As(err, &refused) || refused.Origin != "https://issuer.example.org:443" {
		t.Errorf("move: err = %v, want an egress refusal naming https://issuer.example.org:443", err)
	}
}

// Each change to the sign-in is said in the trail, as booleans: never the
// secret, nor the configuration itself.
func TestWriteMCPServer_aSignInChange_isSaidInTheTrail(t *testing.T) {
	w := withSigninWorld(t)
	if _, err := writeLogin(w, &domain.MCPOAuthLoginPatch{ClientID: "agents", ClientSecret: ptr("s1")}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if _, err := writeLogin(w, &domain.MCPOAuthLoginPatch{ClientID: "agents", ClientSecret: ptr("s2")}); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	var detail map[string]any
	var raw []byte
	if err := w.pool.QueryRow(context.Background(), `
		select detail from admin_events where target = $1 order by event_id desc limit 1`, signin).Scan(&raw); err != nil {
		t.Fatalf("read trail: %v", err)
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail["oauthLoginChanged"] != true || detail["oauthLoginSecretChanged"] != true || detail["hasOAuthLogin"] != true {
		t.Errorf("trail = %v, want the sign-in changed, its secret changed, and held", detail)
	}
	if strings.Contains(string(raw), "s2") || strings.Contains(string(raw), "agents") {
		t.Errorf("trail carries the configuration or the secret: %s", raw)
	}
}

// The worker rewrites the server row when it refreshes the shared grant: it
// must carry the sign-in across, or the server would stop taking personal
// credentials with nobody having decided it.
func TestRefreshMCPServerOAuth_keepsTheServersSignIn(t *testing.T) {
	w := withSigninWorld(t)
	grant := domain.MCPOAuthGrant{AccessToken: "old", RefreshToken: "r", TokenURL: "https://mcp.example.com/token", ExpiresAtUnix: 100}
	if _, err := w.integrations.WriteMCPServer(context.Background(), admin.MCPServerWrite{
		By: "usr_ana", Scope: platform, Server: signinServer(),
		Given: domain.MCPCredentialPatch{OAuth: &grant},
		Login: &domain.MCPOAuthLoginPatch{ClientID: "agents"},
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	stored, err := w.integrations.MCPCredentials(context.Background(), signin)
	if err != nil || stored.OAuth == nil {
		t.Fatalf("stored grant: %+v, %v", stored.OAuth, err)
	}
	next := *stored.OAuth
	next.AccessToken = "new"
	if err := w.integrations.RefreshMCPServerOAuth(context.Background(), admin.MCPOAuthRefresh{
		Server: signin, Was: *stored.OAuth, Next: next,
	}); err != nil {
		t.Fatalf("RefreshMCPServerOAuth: %v", err)
	}
	if got, _ := w.integrations.MCPServerNamed(context.Background(), signin); got.OAuthLogin == nil {
		t.Error("the refresh dropped the sign-in")
	}
}

// A grant got through the sign-in refreshes as the server's client, read at
// each refresh: a secret rotated on the server holds for every person at
// once, and nobody signs in again for it.
func TestMCPOAuthLoginClient_isTheServersConfigurationAndSecretNow(t *testing.T) {
	w := withSigninWorld(t)
	if _, err := writeLogin(w, &domain.MCPOAuthLoginPatch{ClientID: "agents", ClientSecret: ptr("s1")}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if _, err := writeLogin(w, &domain.MCPOAuthLoginPatch{ClientID: "agents", ClientSecret: ptr("s2")}); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	installation := domain.Scope{Company: domain.Installation}
	client, err := w.integrations.MCPOAuthLoginClient(context.Background(), installation, signin)
	if err != nil || client.Login.ClientID != "agents" || client.Secret != "s2" ||
		client.Login.TokenAuthMethod != domain.TokenAuthClientSecretBasic || client.ServerURL != signinServer().URL {
		t.Fatalf("MCPOAuthLoginClient = %+v, %v; want agents, s2, basic, at the server's address", client, err)
	}

	if _, err := writeLogin(w, &domain.MCPOAuthLoginPatch{Remove: true}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := w.integrations.MCPOAuthLoginClient(context.Background(), installation, signin); !errors.Is(err, admin.ErrNoMCPOAuthLogin) {
		t.Errorf("after removal: err = %v, want ErrNoMCPOAuthLogin", err)
	}
}
