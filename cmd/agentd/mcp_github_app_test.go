package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
A remote MCP server reached as a GitHub App.

The fake GitHub verifies the App JWT against the App's public key and issues
installation tokens that expire; the fake MCP server behind it accepts only a
token GitHub issued and has not expired. Between them a test sees what the
hosted GitHub MCP sees.
*/

// installationGuard is what GitHub's MCP endpoint does: a request without a
// live installation token is refused before it reaches a tool.
type installationGuard struct {
	github  *githubapptest.Fake
	next    http.Handler
	served  atomic.Int32
	refused atomic.Int32
}

func (g *installationGuard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !g.github.Valid(token) {
		g.refused.Add(1)
		http.Error(w, "Bad credentials", http.StatusUnauthorized)
		return
	}
	g.served.Add(1)
	g.next.ServeHTTP(w, r)
}

func githubMCP(t *testing.T, github *githubapptest.Fake) (*installationGuard, string) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "github", Version: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "get_me"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "factory-qa[bot]"}}}, nil, nil
		})
	guard := &installationGuard{
		github: github,
		next:   mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil),
	}
	remote := httptest.NewServer(guard)
	t.Cleanup(remote.Close)
	return guard, remote.URL
}

func githubAppCreds(github *githubapptest.Fake) domain.MCPCredentials {
	return domain.MCPCredentials{GitHubApp: &domain.MCPGitHubApp{
		AppID: github.AppID, InstallationID: github.InstallationID,
		PrivateKey: github.PKCS1(), APIBaseURL: github.Base(),
	}}
}

// The installation token is what reaches the MCP server, on discovery and on
// a tool call alike — minted once, not per request.
func TestConnectServer_aGitHubApp_sendsTheInstallationTokenOnMCPCalls(t *testing.T) {
	t.Parallel()
	github := githubapptest.New(t, 5550600, 91)
	guard, url := githubMCP(t, github)
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	server := domain.MCPServer{Name: "github", Transport: domain.TransportHTTP, URL: url, Enabled: true}

	if err := connectServer(t.Context(), catalog, server, githubAppCreds(github), nil, nil,
		credentialPolicy{}, nil); err != nil {
		t.Fatalf("connectServer: %v", err)
	}
	t.Cleanup(func() { _ = catalog.Close() })
	res, err := catalog.Invoke(t.Context(), engine.Call{RunID: "run-1", Seq: 1, Tool: "github.get_me"})

	if err != nil || res.Failed {
		t.Fatalf("tool call = %+v, %v; want it served with the installation token", res, err)
	}
	if guard.refused.Load() != 0 || guard.served.Load() == 0 || github.Minted() != 1 {
		t.Fatalf("served %d, refused %d, minted %d; want every call authorised by one token",
			guard.served.Load(), guard.refused.Load(), github.Minted())
	}
}

type stepClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *stepClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

// Across the token's hour the worker replaces it before GitHub would refuse
// it, so no call on the way ever fails.
func TestAuthenticatedClient_aGitHubAppToken_isRefreshedBeforeExpiryWithNoFailedCall(t *testing.T) {
	t.Parallel()
	github := githubapptest.New(t, 5550600, 91)
	clock := &stepClock{at: time.Now().UTC().Truncate(time.Second)}
	github.UseClock(clock.Now)
	guard := &installationGuard{github: github, next: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}
	remote := httptest.NewServer(guard)
	t.Cleanup(remote.Close)
	client, err := authenticatedClient("github", githubAppCreds(github), nil, nil)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	client.Transport.(*credentialAuth).clock = clock

	for _, step := range []time.Duration{0, 30 * time.Minute, 26 * time.Minute, 30 * time.Minute, 30 * time.Minute} {
		clock.advance(step)
		resp, err := client.Get(remote.URL)
		if err != nil || resp.StatusCode != http.StatusNoContent {
			t.Fatalf("call after %v failed: %v %v", step, err, resp)
		}
		_ = resp.Body.Close()
	}
	if github.Minted() != 3 || guard.refused.Load() != 0 {
		t.Fatalf("minted %d, refused %d; want a refresh before each expiry and nothing refused",
			github.Minted(), guard.refused.Load())
	}
}

type appServers struct {
	server domain.MCPServer
	creds  domain.MCPCredentials
}

func (s appServers) MCPServers(context.Context) ([]domain.MCPServer, error) {
	return []domain.MCPServer{s.server}, nil
}

func (s appServers) MCPCredentials(context.Context, string) (domain.MCPCredentials, error) {
	return s.creds, nil
}

// captureLogs points slog at a buffer for the rest of the test. Not parallel:
// the default logger is process state.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	was := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(was) })
	return &buf
}

// A revoked key and a removed installation both leave the server unreachable,
// each with the reason that says which to fix — and neither the health
// record nor any log line carries the private key or the JWT it signed.
//
// Not parallel: captureLogs replaces the process-wide logger.
func TestReconciler_aRevokedGitHubApp_isUnreachableWithAClearReasonAndNoSecret(t *testing.T) {
	cases := []struct {
		status int
		reason string
	}{
		{http.StatusUnauthorized, "refused the App's JWT"},
		{http.StatusNotFound, "installation not found"},
	}
	for _, tc := range cases {
		logs := captureLogs(t)
		github := githubapptest.New(t, 5550600, 91)
		_, url := githubMCP(t, github)
		github.Refuse(tc.status)
		server := domain.MCPServer{Name: "github", Transport: domain.TransportHTTP, URL: url, Enabled: true}
		creds := githubAppCreds(github)
		health := recordingHealth{}

		newReconciler(tools.NewCatalog(engine.NewMemoryContent()), appServers{server, creds}, health).
			reconcile(t.Context())

		seen := health["github"]
		if seen.Reachable || !strings.Contains(seen.Detail, tc.reason) {
			t.Fatalf("HTTP %d: health = %+v, want unreachable because %q", tc.status, seen, tc.reason)
		}
		// Anchor: the failure was logged, so the absence below is about content.
		if !strings.Contains(logs.String(), "tool server did not answer") || len(github.JWTs()) == 0 {
			t.Fatalf("HTTP %d: no failure logged or no JWT presented; the leak check would prove nothing", tc.status)
		}
		for _, text := range []string{seen.Detail, logs.String()} {
			assertNoAppSecret(t, text, creds.GitHubApp.PrivateKey, github.JWTs())
		}
	}
}

func assertNoAppSecret(t *testing.T, text, key string, jwts []string) {
	t.Helper()
	for _, line := range strings.Split(key, "\n") {
		if len(line) > 16 && strings.Contains(text, line) {
			t.Fatalf("leaked a line of the private key: %s", text)
		}
	}
	for _, jwt := range jwts {
		parts := strings.Split(jwt, ".")
		if len(parts) == 3 && (strings.Contains(text, jwt) || strings.Contains(text, parts[2])) {
			t.Fatalf("leaked the App JWT: %s", text)
		}
	}
}
