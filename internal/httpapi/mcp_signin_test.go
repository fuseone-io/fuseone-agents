package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/mcpauth"
)

/*
A person signing in to a remote MCP server from their browser: the door that
begins it answers where to send the browser, and the return stores their own
credential, marked as got through the sign-in and holding no client secret,
then sends the browser back to Credentials with a code from a closed set.
*/

var signInNow = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

type signInWorld struct {
	s     *Server
	store *admin.Integrations
	idp   *httptest.Server
	asked url.Values
}

func newSignInWorld(t *testing.T) *signInWorld {
	t.Helper()
	w := &signInWorld{s: mcpWorld(t)}
	w.store = w.s.integrations.(*admin.Integrations)
	w.idp = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.asked = r.PostForm
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte(`{"access_token":"person-access","refresh_token":"person-refresh","expires_in":600}`))
	}))
	t.Cleanup(w.idp.Close)
	random := 0
	w.s = w.s.WithMCPSignIn(MCPSignInConfig{
		Store:   w.store,
		Client:  func(domain.MCPServer) *http.Client { return w.idp.Client() },
		BaseURL: "https://agents.example.com",
		Now:     func() time.Time { return signInNow },
		Random: func() (string, error) {
			random++
			return "random-" + string(rune('a'+random)), nil
		},
	})
	w.configure(t, domain.MCPOAuthLoginPatch{
		ClientID: "agents", ClientSecret: ptr("server-secret"), Scopes: []string{"openid"},
		AuthorizeURL: w.idp.URL + "/authorize", TokenURL: w.idp.URL + "/token",
	})
	return w
}

func (w *signInWorld) configure(t *testing.T, login domain.MCPOAuthLoginPatch) {
	t.Helper()
	if _, err := w.store.WriteMCPServer(context.Background(), admin.MCPServerWrite{
		By: "usr_ana", Scope: adminScope,
		Server: domain.MCPServer{Name: "signin", Transport: domain.TransportHTTP, URL: "https://mcp.example.com/signin", Enabled: true},
		Login:  &login,
	}); err != nil {
		t.Fatalf("WriteMCPServer: %v", err)
	}
}

func (w *signInWorld) start(t *testing.T, caller context.Context) url.Values {
	t.Helper()
	resp, err := w.s.StartMCPUserSignIn(caller, openapi.StartMCPUserSignInRequestObject{Name: "signin"})
	ok, isOK := resp.(openapi.StartMCPUserSignIn200JSONResponse)
	if err != nil || !isOK {
		t.Fatalf("StartMCPUserSignIn = %#v, %v", resp, err)
	}
	u, err := url.Parse(ok.AuthorizeURL)
	if err != nil {
		t.Fatalf("authorize URL: %v", err)
	}
	return u.Query()
}

func (w *signInWorld) back(caller context.Context, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(caller, http.MethodGet, "/auth/mcp/callback?"+query, nil)
	rec := httptest.NewRecorder()
	w.s.SignInCallback().ServeHTTP(rec, req)
	return rec
}

func landedOn(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	u, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || u.Path != "/integrations/credentials" {
		t.Fatalf("Location = %q, want Credentials", rec.Header().Get("Location"))
	}
	return u.Query().Get("oauth")
}

func TestStartMCPUserSignIn_answersWhereToSendTheBrowser(t *testing.T) {
	w := newSignInWorld(t)
	q := w.start(t, as(domain.RoleAuthor))
	for field, value := range map[string]string{
		"client_id": "agents", "response_type": "code", "code_challenge_method": "S256", "scope": "openid",
		"redirect_uri": "https://agents.example.com/auth/mcp/callback", "resource": "https://mcp.example.com/signin",
	} {
		if q.Get(field) != value {
			t.Errorf("%s = %q, want %q", field, q.Get(field), value)
		}
	}
	if q.Get("state") == "" || q.Get("code_challenge") == "" {
		t.Errorf("state %q, challenge %q; want both", q.Get("state"), q.Get("code_challenge"))
	}
}

func TestSignInCallback_storesThePersonsOwnCredential_andGoesBackToCredentials(t *testing.T) {
	w := newSignInWorld(t)
	q := w.start(t, as(domain.RoleAuthor))
	rec := w.back(as(domain.RoleAuthor), "code=the-code&state="+url.QueryEscape(q.Get("state")))
	if got := landedOn(t, rec); got != "connected" {
		t.Fatalf("oauth = %q, want connected", got)
	}
	if w.asked.Get("code") != "the-code" || w.asked.Get("code_verifier") == "" || w.asked.Get("resource") != "https://mcp.example.com/signin" {
		t.Errorf("token request = %v", w.asked)
	}
	creds, found, err := w.store.MCPPersonalCredential(context.Background(), domain.Scope{Company: domain.Installation}, "signin", "usr_ana")
	if err != nil || !found || creds.OAuth == nil {
		t.Fatalf("personal credential = %+v, %v, %v", creds, found, err)
	}
	g := creds.OAuth
	if g.AccessToken != "person-access" || g.RefreshToken != "person-refresh" || !g.SignIn ||
		g.ClientSecret != "" || g.ClientID != "agents" || g.ExpiresAtUnix != signInNow.Add(10*time.Minute).Unix() {
		t.Errorf("grant = %+v; want the person's tokens, marked as signed in, no secret held", g)
	}
}

// The answer to a browser coming back says nothing to cache or to pass on.
func TestSignInCallback_isNeitherCachedNorReferred(t *testing.T) {
	w := newSignInWorld(t)
	rec := w.back(as(domain.RoleAuthor), "code=x&state=nothing-in-flight")
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("headers = %v", rec.Header())
	}
	if got := landedOn(t, rec); got != "expired" {
		t.Errorf("oauth = %q, want expired for a sign-in not in flight", got)
	}
}

func TestSignInCallback_refusesWithoutStoringAnything(t *testing.T) {
	cases := map[string]struct {
		back  func(w *signInWorld, state string) *httptest.ResponseRecorder
		want  string
		setup func(t *testing.T, w *signInWorld)
	}{
		"another person's session": {
			back: func(w *signInWorld, state string) *httptest.ResponseRecorder {
				bruno := auth.WithPrincipal(context.Background(), domain.Principal{ID: "usr_bruno", Kind: domain.PrincipalUser,
					Grants: []domain.Grant{{Scope: adminScope, Role: domain.RoleAuthor}}})
				return w.back(bruno, "code=c&state="+url.QueryEscape(state))
			},
			want: "not_yours",
		},
		"the person said no at the provider": {
			back: func(w *signInWorld, state string) *httptest.ResponseRecorder {
				return w.back(as(domain.RoleAuthor), "error=access_denied&error_description=canary&state="+url.QueryEscape(state))
			},
			want: "denied",
		},
		// The same client, so only the row's revision can tell.
		"the server reconfigured since": {
			setup: func(t *testing.T, w *signInWorld) {
				w.configure(t, domain.MCPOAuthLoginPatch{ClientID: "agents", Scopes: []string{"openid", "profile"},
					AuthorizeURL: w.idp.URL + "/authorize", TokenURL: w.idp.URL + "/token"})
			},
			back: func(w *signInWorld, state string) *httptest.ResponseRecorder {
				return w.back(as(domain.RoleAuthor), "code=c&state="+url.QueryEscape(state))
			},
			want: "changed",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			w := newSignInWorld(t)
			q := w.start(t, as(domain.RoleAuthor))
			if c.setup != nil {
				c.setup(t, w)
			}
			if got := landedOn(t, c.back(w, q.Get("state"))); got != c.want {
				t.Errorf("oauth = %q, want %q", got, c.want)
			}
			if _, found, _ := w.store.MCPPersonalCredential(context.Background(), domain.Scope{Company: domain.Installation}, "signin", "usr_ana"); found {
				t.Error("a credential was stored")
			}
			if w.asked != nil {
				t.Errorf("the token endpoint was asked: %v", w.asked)
			}
		})
	}
}

func TestStartMCPUserSignIn_aServerWithNoSignIn_isNotFound(t *testing.T) {
	w := newSignInWorld(t)
	w.configure(t, domain.MCPOAuthLoginPatch{Remove: true})
	resp, err := w.s.StartMCPUserSignIn(as(domain.RoleAuthor), openapi.StartMCPUserSignInRequestObject{Name: "signin"})
	if _, ok := resp.(openapi.StartMCPUserSignIn404ApplicationProblemPlusJSONResponse); err != nil || !ok {
		t.Errorf("StartMCPUserSignIn = %#v, %v; want 404", resp, err)
	}
}

// A browser sent back over plain HTTP would carry the code in the clear.
func TestStartMCPUserSignIn_anInstallationNotOnHTTPS_isRefused(t *testing.T) {
	w := newSignInWorld(t)
	w.s.mcpSignIn.BaseURL = "http://agents.example.com"
	resp, err := w.s.StartMCPUserSignIn(as(domain.RoleAuthor), openapi.StartMCPUserSignInRequestObject{Name: "signin"})
	bad, ok := resp.(openapi.StartMCPUserSignIn400ApplicationProblemPlusJSONResponse)
	if err != nil || !ok || !strings.Contains(valueOr(bad.Detail), "https") {
		raw, _ := json.Marshal(resp)
		t.Errorf("StartMCPUserSignIn = %s, %v; want 400 naming https", raw, err)
	}
}

// RFC 9207: an authorization server that says it names itself must name the
// one the sign-in was found at; one naming another is refused even where it
// need not have named any.
func TestIssuerHolds(t *testing.T) {
	t.Parallel()
	found := admin.MCPOAuthFlow{Issuer: "https://idp.example.com", IssParameter: true}
	optional := admin.MCPOAuthFlow{Issuer: "https://idp.example.com"}
	configured := admin.MCPOAuthFlow{}
	for _, c := range []struct {
		name string
		flow admin.MCPOAuthFlow
		iss  string
		want bool
	}{
		{"the issuer it was found at", found, "https://idp.example.com", true},
		{"another issuer", found, "https://evil.example.com", false},
		{"none, where it says it always names itself", found, "", false},
		{"none, where naming is optional", optional, "", true},
		{"another, where naming is optional", optional, "https://evil.example.com", false},
		{"any, for endpoints configured and not found", configured, "https://idp.example.com", true},
	} {
		if got := issuerHolds(c.flow, c.iss); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

// What the sign-in reaches is held to what the server's own address is: TLS
// unless it is this machine, and the token endpoint inside the egress policy.
func TestEndpointsRefused(t *testing.T) {
	t.Parallel()
	policy := &domain.MCPEgress{Origins: []string{"https://mcp.example.com:443"}}
	inside := mcpauth.Endpoints{AuthorizeURL: "https://idp.example.org/auth", TokenURL: "https://mcp.example.com/token"}
	outside := mcpauth.Endpoints{AuthorizeURL: "https://idp.example.org/auth", TokenURL: "https://idp.example.org/token"}
	clear := mcpauth.Endpoints{AuthorizeURL: "http://idp.example.org/auth", TokenURL: "https://mcp.example.com/token"}
	if p := endpointsRefused(inside, policy); p != nil {
		t.Errorf("inside the policy: %+v", p)
	}
	if p := endpointsRefused(outside, policy); p == nil || valueOr(p.Type) != string(CodeEgressNotAllowed) {
		t.Errorf("outside the policy: %+v, want egress-not-allowed", p)
	}
	if p := endpointsRefused(outside, nil); p != nil {
		t.Errorf("the installation's server, held to no policy: %+v", p)
	}
	if p := endpointsRefused(clear, nil); p == nil {
		t.Error("a plain http endpoint elsewhere was accepted")
	}
	loopback := mcpauth.Endpoints{AuthorizeURL: "http://127.0.0.1:9/auth", TokenURL: "http://localhost:9/token"}
	if p := endpointsRefused(loopback, nil); p != nil {
		t.Errorf("this machine over http: %+v", p)
	}
}
