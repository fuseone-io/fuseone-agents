package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/githubapp"
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
type githubAppAuth struct {
	from   domain.MCPGitHubApp
	source *githubapp.Source
	// err is a credential the worker cannot mint with at all, such as a key
	// that does not parse. Kept so every request reports it the same way.
	err error
}

func (a *credentialAuth) githubAppRoundTrip(
	r *http.Request, app domain.MCPGitHubApp,
) (*http.Response, error) {
	// Checked per request, not at connect: a redirect or an edited URL is
	// still a request the token would ride on.
	if err := githubapp.RequireTLS(r.URL.String()); err != nil {
		return nil, fmt.Errorf("%s: %w", a.server, err)
	}
	current := a.githubApp(app)
	if current.err != nil {
		return nil, fmt.Errorf("%s: %w", a.server, current.err)
	}
	token, err := current.source.Token(r.Context())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", a.server, err)
	}
	return bearer{token: token, base: a.base}.RoundTrip(r)
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
