package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/githubapp"
	"github.com/fuseone/agents/internal/tools"
)

/*
githubAppAuth is a server's GitHub App credential, ready to mint with.

It follows the OAuth path's shape without its persistence. An OAuth refresh
spends a stored refresh token, so the result has to be saved before it is used
and a grant changed underneath has to be detected. An App token is minted from
the stored key and nothing about minting changes what is stored — so the token
stays in this worker's memory, and the only thing to detect is the credential
itself changing: a different App, installation, key or repository list builds
a new source rather than reusing tokens minted for the old one.
*/
// errCrossOriginRedirect is a redirect that would carry the installation token
// to an origin the server was not configured with.
var errCrossOriginRedirect = errors.New(
	"github app: refused a redirect to another origin; the installation token goes only where the server is configured")

type githubAppAuth struct {
	from   domain.MCPGitHubApp
	source *githubapp.Source
	// err is a credential the worker cannot mint with at all, such as a key
	// that does not parse. Kept so every request reports it the same way.
	err error
}

// githubAppRoundTrip sends r as the App. headers are the App's configuration
// — X-MCP-Toolsets on the hosted GitHub MCP — and are set before the bearer,
// so the installation token is always the Authorization that leaves.
func (a *credentialAuth) githubAppRoundTrip(
	r *http.Request, app domain.MCPGitHubApp, headers map[string]string,
) (*http.Response, error) {
	// Checked per request, not at connect: a redirect or an edited URL is
	// still a request the token would ride on.
	if err := githubapp.RequireTLS(r.URL.String()); err != nil {
		return nil, fmt.Errorf("%s: %w", a.server, err)
	}
	if crossOriginHop(r) {
		return nil, fmt.Errorf("%s: %w", a.server, errCrossOriginRedirect)
	}
	current := a.githubApp(app)
	if current.err != nil {
		return nil, fmt.Errorf("%s: %w", a.server, current.err)
	}
	token, err := current.source.Token(r.Context())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", a.server, err)
	}
	return headerAuth{
		headers: githubapp.ConfigHeaders(headers),
		base:    bearer{token: token, base: a.base},
	}.RoundTrip(r)
}

/*
appCredentials is the credential of a server that acts as a GitHub App.

The App is the server's identity, for everyone: no personal credential is
looked up, because a person's token would replace the App's on the wire —
widening what the call may do and breaking the role identity a manual run of
the Reviewer or QA is supposed to carry. A policy that demands a personal
credential on every tool call is the opposite promise, and the App cannot keep
it; the call is refused with that reason rather than sent as either one.
*/
func (a *credentialAuth) appCredentials(ctx context.Context) (domain.MCPCredentials, domain.UserID, error) {
	if a.policy.requirePersonal && tools.IsInvocation(ctx) {
		return domain.MCPCredentials{}, "", credentialFailure(
			tools.CodeMCPPersonalCredentialMissing, a.server,
			"this MCP server acts as a GitHub App, so the policy requiring a personal credential for its tool calls cannot be met", nil)
	}
	return a.shared, "", nil
}

func (a *credentialAuth) githubApp(app domain.MCPGitHubApp) *githubAppAuth {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.app != nil && a.app.from.Equal(app) {
		return a.app
	}
	clock := a.clock
	if clock == nil {
		clock = engine.SystemClock{}
	}
	// Its own client, guarded like every other outbound call: the API base
	// URL is configuration, and configuration can point at metadata.
	mint := &http.Client{Transport: baseHTTPTransport(), Timeout: 30 * time.Second}
	source, err := githubapp.NewSource(app, mint, clock)
	a.app = &githubAppAuth{from: app, source: source, err: err}
	return a.app
}

/*
crossOriginHop reports a redirect that leaves the origin it came from.

The token is set here, on every hop, so Go's rule of dropping Authorization
on a redirect to another host never applies to it. A hop that stays on its
scheme, host and port is the same server and is followed; one that leaves is
refused before a token is minted for it.
*/
func crossOriginHop(r *http.Request) bool {
	if r.Response == nil || r.Response.Request == nil {
		return false
	}
	from := r.Response.Request.URL
	return from.Scheme != r.URL.Scheme || from.Host != r.URL.Host
}
