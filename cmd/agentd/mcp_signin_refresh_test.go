package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/tools"
)

/*
A grant a person got through the server's browser sign-in holds no client
secret: its refresh authenticates as the server's client as it is now, by the
method the server names, and asks for the grant's resource (RFC 8707).
*/

type signInStore struct {
	recordingOAuthStore
	login     domain.MCPOAuthLogin
	secret    string
	serverURL string
}

func (s *signInStore) MCPOAuthLoginClient(context.Context, domain.Scope, string) (admin.SignInClient, error) {
	return admin.SignInClient{Login: s.login, Secret: s.secret, ServerURL: s.serverURL}, nil
}

func TestAuthenticatedClient_aSignInGrant_refreshesAsTheServersClientNow(t *testing.T) {
	t.Parallel()
	store := &signInStore{
		login: domain.MCPOAuthLogin{ClientID: "agents", TokenAuthMethod: domain.TokenAuthClientSecretBasic,
			HasClientSecret: true, Resource: "https://mcp.example.com/mcp"},
		secret: "server-secret", serverURL: "https://mcp.example.com/mcp",
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			want := "Basic " + base64.StdEncoding.EncodeToString([]byte("agents:server-secret"))
			if r.Header.Get("Authorization") != want || r.PostForm.Get("resource") != "https://mcp.example.com/mcp" ||
				r.PostForm.Has("client_secret") {
				t.Errorf("token request: Authorization %q, form %v", r.Header.Get("Authorization"), r.PostForm)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fresh", "expires_in": 3600})
		case "/resource":
			if got := r.Header.Get("Authorization"); got != "Bearer fresh" {
				t.Errorf("Authorization = %q, want the refreshed token", got)
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(remote.Close)
	store.login.TokenURL = remote.URL + "/token"

	grant := domain.MCPOAuthGrant{
		AccessToken: "expired", RefreshToken: "refresh", TokenURL: remote.URL + "/token", ClientID: "agents",
		SignIn: true, SignInConfig: store.login.Identity(remote.URL+"/token", "https://mcp.example.com/mcp"),
		Resource:      "https://mcp.example.com/mcp",
		ExpiresAtUnix: time.Now().Add(-time.Minute).Unix(),
	}
	client, err := authenticatedClient("signin", domain.MCPCredentials{}, store,
		personalCreds{"usr_ana": domain.MCPCredentials{OAuth: &grant}})
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	req, err := http.NewRequestWithContext(tools.WithCaller(t.Context(), "usr_ana"), http.MethodGet, remote.URL+"/resource", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()
	if store.next.AccessToken != "fresh" || store.next.ClientSecret != "" || !store.next.SignIn {
		t.Errorf("persisted %+v; want the fresh token, still a sign-in grant, and no secret held", store.next)
	}
}

// The refresh token belongs to the client it was issued to: a server whose
// sign-in now names another client is told, never asked as the new one.
func TestAuthenticatedClient_aSignInGrantForAnotherClient_isNotRefreshed(t *testing.T) {
	t.Parallel()
	asked := false
	remote := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { asked = true }))
	t.Cleanup(remote.Close)
	store := &signInStore{login: domain.MCPOAuthLogin{ClientID: "agents-2", TokenAuthMethod: domain.TokenAuthNone}}
	grant := domain.MCPOAuthGrant{
		AccessToken: "expired", RefreshToken: "refresh", TokenURL: remote.URL + "/token", ClientID: "agents",
		SignIn: true, ExpiresAtUnix: time.Now().Add(-time.Minute).Unix(),
	}
	client, err := authenticatedClient("signin", domain.MCPCredentials{}, store,
		personalCreds{"usr_ana": domain.MCPCredentials{OAuth: &grant}})
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	req, _ := http.NewRequestWithContext(tools.WithCaller(t.Context(), "usr_ana"), http.MethodGet, remote.URL+"/resource", nil)
	if resp, err := client.Do(req); err == nil {
		_ = resp.Body.Close()
		t.Error("the call went out with a grant the server's client no longer is")
	}
	if asked {
		t.Error("the token endpoint was asked as another client")
	}
}

// The secret now in force must never be sent where the configuration it
// belongs to does not send it: a grant got under another token endpoint,
// with the client kept, asks the person to sign in again.
func TestAuthenticatedClient_aSignInGrantUnderAnotherConfiguration_isNotRefreshed(t *testing.T) {
	t.Parallel()
	asked := false
	old := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { asked = true }))
	t.Cleanup(old.Close)
	now := domain.MCPOAuthLogin{ClientID: "agents", TokenURL: "https://new.example.com/token",
		TokenAuthMethod: domain.TokenAuthClientSecretBasic, HasClientSecret: true}
	then := now
	then.TokenURL = old.URL + "/token"
	store := &signInStore{login: now, secret: "new-secret", serverURL: "https://mcp.example.com/mcp"}
	grant := domain.MCPOAuthGrant{
		AccessToken: "expired", RefreshToken: "refresh", TokenURL: old.URL + "/token", ClientID: "agents",
		SignIn: true, SignInConfig: then.Identity(then.TokenURL, "https://mcp.example.com/mcp"),
		ExpiresAtUnix: time.Now().Add(-time.Minute).Unix(),
	}
	client, err := authenticatedClient("signin", domain.MCPCredentials{}, store,
		personalCreds{"usr_ana": domain.MCPCredentials{OAuth: &grant}})
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	req, _ := http.NewRequestWithContext(tools.WithCaller(t.Context(), "usr_ana"), http.MethodGet, old.URL+"/resource", nil)
	if resp, err := client.Do(req); err == nil {
		_ = resp.Body.Close()
		t.Error("the call went out with a grant of another configuration")
	}
	if asked {
		t.Error("the new secret was sent to the old token endpoint")
	}
}

// discoveredIdP publishes a server's sign-in metadata with a token endpoint the
// test can move, and answers at whichever endpoint the metadata names.
func discoveredIdP(t *testing.T) (*httptest.Server, *string, *[]string) {
	t.Helper()
	tokenPath, asked := "/token-v1", &[]string{}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/mcp":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": srv.URL + "/mcp", "authorization_servers": []string{srv.URL}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": srv.URL, "authorization_endpoint": srv.URL + "/auth",
				"token_endpoint": srv.URL + tokenPath, "code_challenge_methods_supported": []string{"S256"}})
		case "/token-v1", "/token-v2":
			*asked = append(*asked, r.URL.Path)
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fresh", "expires_in": 60})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &tokenPath, asked
}

func discoveredGrant(srv *httptest.Server, login domain.MCPOAuthLogin) domain.MCPOAuthGrant {
	return domain.MCPOAuthGrant{
		AccessToken: "expired", RefreshToken: "refresh", TokenURL: srv.URL + "/token-v1", ClientID: "agents",
		SignIn: true, SignInConfig: login.Identity(srv.URL+"/token-v1", srv.URL+"/mcp"), Resource: srv.URL + "/mcp",
		ExpiresAtUnix: time.Now().Add(-time.Minute).Unix(),
	}
}

func callAs(t *testing.T, client *http.Client, at string) error {
	t.Helper()
	req, _ := http.NewRequestWithContext(tools.WithCaller(t.Context(), "usr_ana"), http.MethodGet, at, nil)
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	return err
}

// Found by discovery, the token endpoint is resolved again at each refresh:
// metadata that has moved it since the grant was got sends nothing, the
// rotated secret least of all, to the endpoint the grant was got at.
func TestAuthenticatedClient_aSignInGrantWhoseFoundEndpointMoved_isNotRefreshed(t *testing.T) {
	t.Parallel()
	srv, tokenPath, asked := discoveredIdP(t)
	login := domain.MCPOAuthLogin{ClientID: "agents", TokenAuthMethod: domain.TokenAuthClientSecretBasic, HasClientSecret: true}
	store := &signInStore{login: login, secret: "rotated", serverURL: srv.URL + "/mcp"}
	grant := discoveredGrant(srv, login)
	*tokenPath = "/token-v2"
	client, err := authenticatedClient("signin", domain.MCPCredentials{}, store,
		personalCreds{"usr_ana": domain.MCPCredentials{OAuth: &grant}})
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	if err := callAs(t, client, srv.URL+"/resource"); err == nil {
		t.Error("the call went out with a grant got at an endpoint the metadata no longer names")
	}
	if len(*asked) != 0 {
		t.Errorf("token endpoints asked: %v; want none", *asked)
	}
}

// The control: metadata unchanged, the grant refreshes at the endpoint found.
func TestAuthenticatedClient_aSignInGrantWhoseFoundEndpointHolds_refreshesThere(t *testing.T) {
	t.Parallel()
	srv, _, asked := discoveredIdP(t)
	login := domain.MCPOAuthLogin{ClientID: "agents", TokenAuthMethod: domain.TokenAuthNone}
	store := &signInStore{login: login, serverURL: srv.URL + "/mcp"}
	grant := discoveredGrant(srv, login)
	client, err := authenticatedClient("signin", domain.MCPCredentials{}, store,
		personalCreds{"usr_ana": domain.MCPCredentials{OAuth: &grant}})
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	if err := callAs(t, client, srv.URL+"/resource"); err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(*asked) != 1 || (*asked)[0] != "/token-v1" {
		t.Errorf("token endpoints asked: %v; want /token-v1 once", *asked)
	}
}
