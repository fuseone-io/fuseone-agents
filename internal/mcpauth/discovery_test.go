package mcpauth_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/mcpauth"
)

/*
Where a server's sign-in goes, found as an MCP client finds it: the server's
protected resource metadata names its authorization server (RFC 9728), and
that server's metadata names its endpoints (RFC 8414, or OpenID discovery
appended to an issuer with a path, as a Keycloak realm has). What the
administrator configured wins over what is found.
*/

func discoveryWorld(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issuer := srv.URL + "/realms/agents"
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/mcp":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource": srv.URL + "/mcp", "authorization_servers": []string{issuer},
			})
		case "/realms/agents/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": issuer, "authorization_endpoint": issuer + "/auth", "token_endpoint": issuer + "/token",
				"code_challenge_methods_supported":               []string{"S256"},
				"authorization_response_iss_parameter_supported": true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, srv.URL + "/mcp"
}

func TestDiscover_findsTheEndpointsFromTheServersMetadata(t *testing.T) {
	t.Parallel()
	srv, server := discoveryWorld(t)
	got, err := mcpauth.Discover(t0(), srv.Client(), server, domain.MCPOAuthLogin{ClientID: "agents"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	issuer := srv.URL + "/realms/agents"
	want := mcpauth.Endpoints{Issuer: issuer, AuthorizeURL: issuer + "/auth", TokenURL: issuer + "/token", IssParameter: true}
	if got != want {
		t.Errorf("Discover = %+v, want %+v", got, want)
	}
}

func TestDiscover_whatTheAdministratorConfigured_wins(t *testing.T) {
	t.Parallel()
	srv, server := discoveryWorld(t)
	got, err := mcpauth.Discover(t0(), srv.Client(), server, domain.MCPOAuthLogin{
		ClientID: "agents", TokenURL: "https://relay.example.com/token",
	})
	if err != nil || got.TokenURL != "https://relay.example.com/token" || got.AuthorizeURL != srv.URL+"/realms/agents/auth" {
		t.Errorf("Discover = %+v, %v; want the configured token endpoint and the found authorize one", got, err)
	}
}

// Both endpoints configured: nothing is fetched, and no issuer is claimed.
func TestDiscover_bothEndpointsConfigured_fetchesNothing(t *testing.T) {
	t.Parallel()
	fetched := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { fetched = true }))
	t.Cleanup(srv.Close)
	got, err := mcpauth.Discover(t0(), srv.Client(), srv.URL+"/mcp", domain.MCPOAuthLogin{
		ClientID: "agents", AuthorizeURL: "https://idp.example.com/auth", TokenURL: "https://idp.example.com/token",
	})
	if err != nil || fetched || got.Issuer != "" {
		t.Errorf("Discover = %+v, %v, fetched %v; want the configuration alone", got, err, fetched)
	}
}

func TestDiscover_aServerThatPublishesNothing_isADiscoveryFailure(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	if _, err := mcpauth.Discover(t0(), srv.Client(), srv.URL+"/mcp", domain.MCPOAuthLogin{ClientID: "agents"}); !errors.Is(err, mcpauth.ErrDiscovery) {
		t.Errorf("err = %v, want ErrDiscovery", err)
	}
}

func TestAuthorizeURL_asksForACodeWithPKCEStateAndResource(t *testing.T) {
	t.Parallel()
	raw := mcpauth.AuthorizeURL("https://idp.example.com/auth?p=signin", mcpauth.Authorization{
		ClientID: "agents", Scopes: []string{"openid", "offline_access"},
		RedirectURI: "https://agents.example.com/auth/mcp/callback",
		State:       "st", Verifier: "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
		Resource: "https://mcp.example.com/mcp",
	})
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	for field, value := range map[string]string{
		"p": "signin", "response_type": "code", "client_id": "agents", "scope": "openid offline_access",
		"redirect_uri": "https://agents.example.com/auth/mcp/callback", "state": "st",
		// RFC 7636 appendix B: the verifier's S256 challenge.
		"code_challenge": "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", "code_challenge_method": "S256",
		"resource": "https://mcp.example.com/mcp",
	} {
		if q.Get(field) != value {
			t.Errorf("%s = %q, want %q", field, q.Get(field), value)
		}
	}
}

// RFC 9728 §3.1: a document at the origin's well-known address describes the
// origin. A server that publishes only there is found, its metadata held to
// the origin as the resource, not to the server's own path.
func TestDiscover_aServerPublishingOnlyAtItsOrigin_isFound(t *testing.T) {
	t.Parallel()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issuer := srv.URL
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": srv.URL, "authorization_servers": []string{issuer}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": issuer, "authorization_endpoint": issuer + "/auth", "token_endpoint": issuer + "/token",
				"code_challenge_methods_supported": []string{"S256"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	got, err := mcpauth.Discover(t0(), srv.Client(), srv.URL+"/mcp", domain.MCPOAuthLogin{ClientID: "agents"})
	if err != nil || got.TokenURL != srv.URL+"/token" {
		t.Errorf("Discover = %+v, %v; want the origin's metadata", got, err)
	}
}

// The path's own document is still held to the server's address: one that
// names another resource is not taken for this server's.
func TestDiscover_thePathsDocumentNamingAnotherResource_isNotTaken(t *testing.T) {
	t.Parallel()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/.well-known/oauth-protected-resource/mcp" {
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": srv.URL + "/other", "authorization_servers": []string{srv.URL}})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	if _, err := mcpauth.Discover(t0(), srv.Client(), srv.URL+"/mcp", domain.MCPOAuthLogin{ClientID: "agents"}); !errors.Is(err, mcpauth.ErrDiscovery) {
		t.Errorf("err = %v, want ErrDiscovery", err)
	}
}
