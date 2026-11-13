package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/githubapp/githubapptest"
	"github.com/fuseone/agents/internal/tools"
)

/*
A server outside the installation sends requests only to the origins the
platform allows its scope (NT-019, decided 2026-11-13).

netguard keeps every server off cloud metadata and link-local addresses, and
nothing more: private addresses stay reachable. So the worker holds an area's
server to the platform's list on the connection, on every redirect, and on the
token endpoints its credentials call, and a server with no list reaches
nothing. The installation's servers carry no list and keep their reach.
*/

var egressArea = domain.Scope{Company: "acme", Area: "cx"}

// countingServer counts every request that reaches it.
func countingServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	hits := new(atomic.Int64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if h != nil {
			h(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, hits
}

func mcpOrigin(t *testing.T, raw string) string {
	t.Helper()
	origin, err := domain.CanonicalOrigin(raw)
	if err != nil {
		t.Fatalf("origin of %q: %v", raw, err)
	}
	return origin
}

// mcpHandler is a working MCP server with one tool.
func mcpHandler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "egress-test", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ping"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{}, nil, nil
		})
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
}

func areaServer(rawURL string, egress *domain.MCPEgress) domain.MCPServer {
	return domain.MCPServer{
		Name: "jira", Transport: domain.TransportHTTP, URL: rawURL, Enabled: true,
		Scope: egressArea, Egress: egress,
	}
}

func TestConnectServer_anAreasServerWithNoPolicy_sendsNothing(t *testing.T) {
	t.Parallel()
	remote, hits := countingServer(t, nil)
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	err := connectServer(t.Context(), catalog, areaServer(remote.URL, &domain.MCPEgress{}),
		domain.MCPCredentials{}, nil, nil, credentialPolicy{}, nil)
	if err == nil || hits.Load() != 0 {
		t.Errorf("err = %v, hits = %d; want refused before any request", err, hits.Load())
	}
}

func TestConnectServer_anAreasServerOutsideItsPolicy_sendsNothing(t *testing.T) {
	t.Parallel()
	remote, hits := countingServer(t, nil)
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	err := connectServer(t.Context(), catalog,
		areaServer(remote.URL, &domain.MCPEgress{Origins: []string{"https://jira.example.com:443"}}),
		domain.MCPCredentials{}, nil, nil, credentialPolicy{}, nil)
	if err == nil || hits.Load() != 0 {
		t.Errorf("err = %v, hits = %d; want refused before any request", err, hits.Load())
	}
}

func TestConnectServer_anAreasServerInsideItsPolicy_connects(t *testing.T) {
	t.Parallel()
	remote := httptest.NewServer(mcpHandler())
	t.Cleanup(remote.Close)
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	err := connectServer(t.Context(), catalog,
		areaServer(remote.URL, &domain.MCPEgress{Origins: []string{mcpOrigin(t, remote.URL)}}),
		domain.MCPCredentials{}, nil, nil, credentialPolicy{}, nil)
	if err != nil {
		t.Fatalf("connectServer: %v", err)
	}
	if _, ok := catalog.Effect("jira.ping"); !ok {
		t.Error("jira.ping was not discovered")
	}
}

// A redirect is a new request: one to an origin outside the list is not sent.
func TestConnectServer_aRedirectOutsideThePolicy_isNotFollowed(t *testing.T) {
	t.Parallel()
	elsewhere, elsewhereHits := countingServer(t, nil)
	allowed, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/mcp", http.StatusTemporaryRedirect)
	})
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	err := connectServer(t.Context(), catalog,
		areaServer(allowed.URL, &domain.MCPEgress{Origins: []string{mcpOrigin(t, allowed.URL)}}),
		domain.MCPCredentials{}, nil, nil, credentialPolicy{}, nil)
	if err == nil || elsewhereHits.Load() != 0 {
		t.Errorf("err = %v, redirected hits = %d; want the redirect refused", err, elsewhereHits.Load())
	}
}

// The token endpoint a grant refreshes at is a request too.
func TestOAuthRefresh_atATokenEndpointOutsideThePolicy_isNotSent(t *testing.T) {
	t.Parallel()
	tokens, tokenHits := countingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fresh", "expires_in": 3600})
	})
	remote, _ := countingServer(t, nil)
	grant := domain.MCPOAuthGrant{
		AccessToken: "expired", RefreshToken: "refresh", TokenURL: tokens.URL + "/token",
		ExpiresAtUnix: time.Now().Add(-time.Minute).Unix(),
	}
	server := areaServer(remote.URL, &domain.MCPEgress{Origins: []string{mcpOrigin(t, remote.URL)}})
	client, err := authenticatedClientWithPolicy(server, domain.MCPCredentials{OAuth: &grant}, nil, nil, credentialPolicy{})
	if err != nil {
		t.Fatalf("authenticatedClientWithPolicy: %v", err)
	}
	if resp, err := client.Get(remote.URL + "/mcp"); err == nil {
		_ = resp.Body.Close()
		t.Error("the request went out with a grant refreshed outside the policy")
	}
	if tokenHits.Load() != 0 {
		t.Errorf("token endpoint hits = %d, want none", tokenHits.Load())
	}
}

// A GitHub App mints its installation token at its API base: outside the
// list, it is not asked.
func TestGitHubAppMint_atAnAPIBaseOutsideThePolicy_isNotSent(t *testing.T) {
	t.Parallel()
	github := githubapptest.New(t, 5550600, 91)
	remote, _ := countingServer(t, nil)
	server := areaServer(remote.URL, &domain.MCPEgress{Origins: []string{mcpOrigin(t, remote.URL)}})
	client, err := authenticatedClientWithPolicy(server, githubAppCreds(github), nil, nil, credentialPolicy{})
	if err != nil {
		t.Fatalf("authenticatedClientWithPolicy: %v", err)
	}
	if resp, err := client.Get(remote.URL + "/mcp"); err == nil {
		_ = resp.Body.Close()
		t.Error("the request went out with a token minted outside the policy")
	}
	if github.Minted() != 0 {
		t.Errorf("minted = %d, want none", github.Minted())
	}
}

// The installation's server carries no list and keeps its reach, redirects
// included.
func TestConnectServer_theInstallationsServer_isNotHeldToAPolicy(t *testing.T) {
	t.Parallel()
	remote := httptest.NewServer(mcpHandler())
	t.Cleanup(remote.Close)
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	server := domain.MCPServer{Name: "jira", Transport: domain.TransportHTTP, URL: remote.URL, Enabled: true}
	if err := connectServer(t.Context(), catalog, server, domain.MCPCredentials{}, nil, nil, credentialPolicy{}, nil); err != nil {
		t.Fatalf("connectServer: %v", err)
	}
}

// A narrowed policy is a changed server: the worker connects it again under
// the new list rather than keeping a session opened under the old one.
func TestFingerprint_changesWithThePolicy(t *testing.T) {
	t.Parallel()
	u, _ := url.Parse("https://jira.example.com/mcp")
	wide := areaServer(u.String(), &domain.MCPEgress{Origins: []string{"https://jira.example.com:443", "https://other.example.com:443"}})
	narrow := areaServer(u.String(), &domain.MCPEgress{Origins: []string{"https://jira.example.com:443"}})
	none := areaServer(u.String(), &domain.MCPEgress{})
	installation := areaServer(u.String(), nil)
	installation.Scope = domain.Scope{Company: domain.Installation}
	seen := map[string]bool{}
	for _, s := range []domain.MCPServer{wide, narrow, none, installation} {
		seen[fingerprint(s)] = true
	}
	if len(seen) != 4 {
		t.Errorf("%d distinct fingerprints for four policies, want 4", len(seen))
	}
}

// The URL is judged before a transport exists, not only when the first
// request leaves: each layer holds on its own.
func TestTransportFor_anAreasServerOutsideItsPolicy_isRefusedBeforeATransportExists(t *testing.T) {
	t.Parallel()
	for name, egress := range map[string]*domain.MCPEgress{
		"no policy":      {},
		"another origin": {Origins: []string{"https://other.example.com:443"}},
	} {
		transport, _, err := transportFor(t.Context(), areaServer("https://jira.example.com/mcp", egress),
			domain.MCPCredentials{}, nil, nil, credentialPolicy{}, nil)
		if err == nil || transport != nil {
			t.Errorf("%s: transport = %v, err = %v; want refused", name, transport, err)
		}
	}
}

/*
An area's server whose policy got lost on the way is refused, not treated as
the installation's: only a server with no scope, or the installation's, may
carry no policy.
*/
func TestEgress_anAreasServerWithItsPolicyLost_isRefusedNotUnrestricted(t *testing.T) {
	t.Parallel()
	remote, hits := countingServer(t, nil)
	lost := areaServer(remote.URL, nil)
	if transport, _, err := transportFor(t.Context(), lost, domain.MCPCredentials{}, nil, nil, credentialPolicy{}, nil); err == nil {
		t.Errorf("transportFor = %v, nil; want refused", transport)
	}
	if resp, err := withEgress(lost, nil).Get(remote.URL); err == nil {
		_ = resp.Body.Close()
		t.Error("the client sent a request for an area's server with no policy")
	}
	if hits.Load() != 0 {
		t.Errorf("hits = %d, want none", hits.Load())
	}
}
