package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	login  domain.MCPOAuthLogin
	secret string
}

func (s *signInStore) MCPOAuthLoginClient(context.Context, domain.Scope, string) (domain.MCPOAuthLogin, string, error) {
	return s.login, s.secret, nil
}

func TestAuthenticatedClient_aSignInGrant_refreshesAsTheServersClientNow(t *testing.T) {
	t.Parallel()
	store := &signInStore{
		login:  domain.MCPOAuthLogin{ClientID: "agents", TokenAuthMethod: domain.TokenAuthClientSecretBasic, HasClientSecret: true},
		secret: "server-secret",
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

	grant := domain.MCPOAuthGrant{
		AccessToken: "expired", RefreshToken: "refresh", TokenURL: remote.URL + "/token", ClientID: "agents",
		SignIn: true, Resource: "https://mcp.example.com/mcp", ExpiresAtUnix: time.Now().Add(-time.Minute).Unix(),
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
