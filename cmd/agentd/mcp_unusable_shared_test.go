package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/tools"
)

/*
A server that takes each person's own credential is not taken down by a broken
shared one.

The shared grant only discovers such a server. Refused while building the
client, as it was, it took every call down with it, a person's own working
credential included — and the message named the grant nobody was calling with.
The refusal now belongs to the requests that would have carried it.
*/

// What a save stored: the client typed in, no token.
var unusableShared = domain.MCPCredentials{OAuth: &domain.MCPOAuthGrant{
	ClientID: "agents", TokenURL: "https://idp.example.com/token",
}}

func TestAuthenticatedClient_aBrokenSharedGrant_leavesAPersonsOwnCredentialWorking(t *testing.T) {
	t.Parallel()
	var seen string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(remote.Close)

	client, err := authenticatedClientWithPolicy(domain.MCPServer{Name: "dpg"}, unusableShared,
		nil, personalCreds{"usr_ana": domain.MCPCredentials{Token: "ana"}},
		credentialPolicy{requirePersonal: true})
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	req, err := http.NewRequestWithContext(tools.WithInvocation(tools.WithCaller(t.Context(), "usr_ana")),
		http.MethodGet, remote.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("a call on Ana's behalf: %v", err)
	}
	_ = resp.Body.Close()
	if seen != "Bearer ana" {
		t.Fatalf("Authorization = %q, want Ana's own credential", seen)
	}
}

func TestAuthenticatedClient_aBrokenSharedGrant_refusesTheDiscoveryThatWouldCarryIt(t *testing.T) {
	t.Parallel()
	remote := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("discovery reached the server with a grant that has nothing to send")
	}))
	t.Cleanup(remote.Close)

	client, err := authenticatedClientWithPolicy(domain.MCPServer{Name: "dpg"}, unusableShared,
		nil, personalCreds{}, credentialPolicy{requirePersonal: true})
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	_, err = client.Get(remote.URL)
	if err == nil || !strings.Contains(err.Error(), "no access token and cannot refresh") {
		t.Fatalf("discovery error = %v, want the shared grant named as unusable", err)
	}
}
