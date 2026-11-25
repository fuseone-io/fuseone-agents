package mcpauth_test

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/mcpauth"
)

/*
The token endpoint, as a browser sign-in and the worker's refresh reach it:
the client authenticated as its configuration says, the resource named on
every request (RFC 8707), no redirect followed, and a refusal told by a closed
code, never by what the endpoint wrote.
*/

type seen struct {
	form   url.Values
	header http.Header
}

func endpoint(t *testing.T, status int, body string) (*httptest.Server, *seen) {
	t.Helper()
	got := &seen{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		got.form, got.header = r.PostForm, r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

const granted = `{"access_token":"at","refresh_token":"rt","token_type":"Bearer","expires_in":300,"scope":"openid mcp"}`

func exchange(srv *httptest.Server, client mcpauth.Client) (mcpauth.Tokens, error) {
	return mcpauth.ExchangeCode(t0(), srv.Client(), mcpauth.CodeExchange{
		Token:       mcpauth.TokenRequest{TokenURL: srv.URL + "/token", Client: client, Resource: "https://mcp.example.com/mcp"},
		Code:        "the-code",
		Verifier:    "the-verifier",
		RedirectURI: "https://agents.example.com/auth/mcp/callback",
	})
}

func TestExchangeCode_clientSecretBasic_authenticatesInTheHeaderAlone(t *testing.T) {
	t.Parallel()
	srv, got := endpoint(t, http.StatusOK, granted)
	tokens, err := exchange(srv, mcpauth.Client{ID: "agents:one", Secret: "s/3 cret", Method: "client_secret_basic"})
	if err != nil || tokens.AccessToken != "at" || tokens.RefreshToken != "rt" || tokens.ExpiresIn != 300 {
		t.Fatalf("ExchangeCode = %+v, %v", tokens, err)
	}
	// RFC 6749 §2.3.1: each half form-encoded before the colon joins them.
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(url.QueryEscape("agents:one")+":"+url.QueryEscape("s/3 cret")))
	if got.header.Get("Authorization") != want {
		t.Errorf("Authorization = %q, want %q", got.header.Get("Authorization"), want)
	}
	if got.form.Has("client_secret") {
		t.Error("the secret was also sent in the body")
	}
	for field, value := range map[string]string{
		"grant_type": "authorization_code", "code": "the-code", "code_verifier": "the-verifier",
		"redirect_uri": "https://agents.example.com/auth/mcp/callback", "resource": "https://mcp.example.com/mcp",
	} {
		if got.form.Get(field) != value {
			t.Errorf("%s = %q, want %q", field, got.form.Get(field), value)
		}
	}
}

func TestExchangeCode_clientSecretPost_sendsTheClientInTheBody(t *testing.T) {
	t.Parallel()
	srv, got := endpoint(t, http.StatusOK, granted)
	if _, err := exchange(srv, mcpauth.Client{ID: "agents", Secret: "s3cret", Method: "client_secret_post"}); err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if got.form.Get("client_id") != "agents" || got.form.Get("client_secret") != "s3cret" || got.header.Get("Authorization") != "" {
		t.Errorf("form = %v, Authorization %q; want the client in the body alone", got.form, got.header.Get("Authorization"))
	}
}

func TestExchangeCode_aPublicClient_sendsItsIDAndNoSecret(t *testing.T) {
	t.Parallel()
	srv, got := endpoint(t, http.StatusOK, granted)
	if _, err := exchange(srv, mcpauth.Client{ID: "agents", Secret: "never-sent", Method: "none"}); err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if got.form.Get("client_id") != "agents" || got.form.Has("client_secret") || got.header.Get("Authorization") != "" {
		t.Errorf("form = %v, Authorization %q; want the client ID alone", got.form, got.header.Get("Authorization"))
	}
}

func TestRefresh_asksWithTheRefreshTokenAndTheResource(t *testing.T) {
	t.Parallel()
	srv, got := endpoint(t, http.StatusOK, `{"access_token":"at2","expires_in":60}`)
	tokens, err := mcpauth.Refresh(t0(), srv.Client(), mcpauth.TokenRequest{
		TokenURL: srv.URL + "/token", Client: mcpauth.Client{ID: "agents", Method: "none"},
		Resource: "https://mcp.example.com/mcp",
	}, "rt")
	if err != nil || tokens.AccessToken != "at2" {
		t.Fatalf("Refresh = %+v, %v", tokens, err)
	}
	if got.form.Get("grant_type") != "refresh_token" || got.form.Get("refresh_token") != "rt" ||
		got.form.Get("resource") != "https://mcp.example.com/mcp" {
		t.Errorf("form = %v", got.form)
	}
}

// What the endpoint writes in a refusal is its own text, and may quote what
// was sent: the error carries a code from a closed set and nothing else.
func TestExchangeCode_aRefusal_isAClosedCodeWithoutTheEndpointsWords(t *testing.T) {
	t.Parallel()
	const canary = "canary-9b1e"
	for raw, want := range map[string]string{
		`{"error":"invalid_grant","error_description":"` + canary + `"}`: "invalid_grant",
		`{"error":"` + canary + `"}`:                                     "refused",
		`<html>` + canary + `</html>`:                                    "refused",
	} {
		srv, _ := endpoint(t, http.StatusBadRequest, raw)
		_, err := exchange(srv, mcpauth.Client{ID: "agents", Method: "none"})
		var refused *mcpauth.TokenRefusal
		if !errors.As(err, &refused) || refused.Code != want {
			t.Errorf("%s: err = %v, want a refusal coded %s", raw, err, want)
		}
		if err != nil && strings.Contains(err.Error(), canary) {
			t.Errorf("%s: the error repeats the endpoint: %v", raw, err)
		}
	}
}

func TestExchangeCode_followsNoRedirect(t *testing.T) {
	t.Parallel()
	elsewhere := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhere = true
		_, _ = w.Write([]byte(granted))
	}))
	t.Cleanup(target.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	if _, err := exchange(srv, mcpauth.Client{ID: "agents", Method: "none"}); err == nil || elsewhere {
		t.Errorf("err = %v, redirected = %v; want refused there, the code sent nowhere else", err, elsewhere)
	}
}

func TestExchangeCode_refusesAnAnswerWithNoAccessToken_orTooLarge(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"no access token": `{"token_type":"Bearer"}`,
		"too large":       `{"access_token":"` + strings.Repeat("a", 2<<20) + `"}`,
	} {
		srv, _ := endpoint(t, http.StatusOK, body)
		if _, err := exchange(srv, mcpauth.Client{ID: "agents", Method: "none"}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
