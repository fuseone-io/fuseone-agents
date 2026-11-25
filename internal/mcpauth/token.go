/*
Package mcpauth is the OAuth client a remote MCP server's browser sign-in
needs: where its endpoints are, the address the browser opens, and the token
endpoint, reached for a code and for every refresh after it.

It knows nothing of the HTTP API or the store. Its callers hand it an
http.Client that already holds the installation's network guard and the
server's egress policy; it adds what OAuth itself asks of a client.
*/
package mcpauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// How the token endpoint is told which client asks, as a server's sign-in
// configuration names it (domain.TokenAuth*).
const (
	MethodNone  = "none"
	MethodPost  = "client_secret_post"
	MethodBasic = "client_secret_basic"
)

// maxTokenAnswer bounds what is read of a token endpoint's answer.
const maxTokenAnswer = 1 << 20

// Client is who asks the token endpoint, and how it proves it.
type Client struct {
	ID     string
	Secret string
	Method string
}

// TokenRequest is one request to a token endpoint: where, by which client,
// for which resource (RFC 8707; empty sends none).
type TokenRequest struct {
	TokenURL string
	Client   Client
	Resource string
}

// CodeExchange is the request that turns a browser sign-in's code into tokens.
type CodeExchange struct {
	Token       TokenRequest
	Code        string
	Verifier    string
	RedirectURI string
}

// Tokens is what the endpoint granted. ExpiresIn is seconds from the answer,
// zero when the endpoint said nothing: the caller dates it by its own clock.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    int64
	Scopes       []string
}

// ErrTokenRefused is a token endpoint that answered but granted nothing.
var ErrTokenRefused = errors.New("mcpauth: the token endpoint refused")

// TokenRefusal is ErrTokenRefused with the endpoint's error code, when it is
// one RFC 6749 §5.2 names, and "refused" for anything else. Nothing the
// endpoint wrote is carried: a description may quote what was sent.
type TokenRefusal struct {
	Code string
}

func (r *TokenRefusal) Error() string { return ErrTokenRefused.Error() + ": " + r.Code }

func (r *TokenRefusal) Is(target error) bool { return target == ErrTokenRefused }

var namedRefusals = []string{
	"invalid_request", "invalid_client", "invalid_grant",
	"unauthorized_client", "unsupported_grant_type", "invalid_scope", "invalid_target",
}

// ExchangeCode asks the token endpoint for the tokens a sign-in's code is for.
func ExchangeCode(ctx context.Context, hc *http.Client, x CodeExchange) (Tokens, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", x.Code)
	form.Set("code_verifier", x.Verifier)
	form.Set("redirect_uri", x.RedirectURI)
	return requestTokens(ctx, hc, x.Token, form)
}

// Refresh asks the token endpoint for a new access token.
func Refresh(ctx context.Context, hc *http.Client, r TokenRequest, refreshToken string) (Tokens, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	return requestTokens(ctx, hc, r, form)
}

func requestTokens(ctx context.Context, hc *http.Client, r TokenRequest, form url.Values) (Tokens, error) {
	if r.Resource != "" {
		form.Set("resource", r.Resource)
	}
	header, err := authenticate(r.Client, form)
	if err != nil {
		return Tokens{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, fmt.Errorf("mcpauth: token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	resp, err := withoutRedirects(hc).Do(req)
	if err != nil {
		return Tokens{}, fmt.Errorf("mcpauth: token endpoint: %w", err)
	}
	defer resp.Body.Close()
	return readTokens(resp)
}

// authenticate puts the client where its method says, and answers the
// Authorization header to send, if any.
func authenticate(c Client, form url.Values) (string, error) {
	switch c.Method {
	case MethodBasic:
		// RFC 6749 §2.3.1: each half form-encoded before the colon joins them.
		pair := url.QueryEscape(c.ID) + ":" + url.QueryEscape(c.Secret)
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(pair)), nil
	case MethodPost:
		form.Set("client_id", c.ID)
		form.Set("client_secret", c.Secret)
		return "", nil
	case MethodNone, "":
		form.Set("client_id", c.ID)
		return "", nil
	}
	return "", fmt.Errorf("mcpauth: no token endpoint method %q", c.Method)
}

// withoutRedirects is the caller's client, its guards kept, refusing to
// follow a redirect: a code or a secret must reach the endpoint configured
// and nowhere it points to.
func withoutRedirects(hc *http.Client) *http.Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	out := *hc
	out.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &out
}

func readTokens(resp *http.Response) (Tokens, error) {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenAnswer+1))
	if err != nil {
		return Tokens{}, fmt.Errorf("mcpauth: read the token answer: %w", err)
	}
	if len(raw) > maxTokenAnswer {
		return Tokens{}, errors.New("mcpauth: the token answer is larger than any token")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Tokens{}, refusalOf(raw)
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return Tokens{}, errors.New("mcpauth: the token answer is not JSON")
	}
	if body.AccessToken == "" {
		return Tokens{}, errors.New("mcpauth: the token answer has no access token")
	}
	return Tokens{
		AccessToken: body.AccessToken, RefreshToken: body.RefreshToken,
		TokenType: strings.TrimSpace(body.TokenType), ExpiresIn: max(body.ExpiresIn, 0),
		Scopes: strings.Fields(body.Scope),
	}, nil
}

func refusalOf(raw []byte) error {
	var body struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &body) == nil && slices.Contains(namedRefusals, body.Error) {
		return &TokenRefusal{Code: body.Error}
	}
	return &TokenRefusal{Code: "refused"}
}
