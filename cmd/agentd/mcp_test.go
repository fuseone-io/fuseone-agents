package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/netguard"
	"github.com/fuseone/agents/internal/tools"
)

/*
What a locally executed tool server is handed.

A stdio server is not another transport. It is a program this installation
starts inside the worker, and until now it started with `cmd.Env` unset — which
in Go means the child inherits everything the worker holds. The worker holds
the database URL and the master key.

So the Gate governed what the *tool* could do while the *process* could read
the key and open the database itself, with no tool call, no ledger step and no
decision. Scrubbing the environment does not make stdio safe — it is still code
running as the worker — but it stops the platform from handing over its own
credentials to do it with.
*/
func TestCommandFor_aStdioServer_doesNotInheritTheWorkersEnvironment(t *testing.T) {
	t.Setenv("FUSEONE_MASTER_KEY", "must-not-travel")
	t.Setenv("DATABASE_URL", "postgres://must-not-travel")
	t.Setenv("PATH", "/usr/bin:/bin")

	cmd, cleanup, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	t.Cleanup(cleanup)

	if cmd.Env == nil {
		t.Fatal("Env is nil, which is how the child inherits everything")
	}
	for _, secret := range []string{"FUSEONE_MASTER_KEY", "DATABASE_URL"} {
		if i := slices.IndexFunc(cmd.Env, func(v string) bool {
			return strings.HasPrefix(v, secret+"=")
		}); i >= 0 {
			t.Errorf("the child is handed %s", secret)
		}
	}

	// And the other direction, or an empty Env would pass this test while
	// leaving a server unable to find the binary it was told to run.
	if !slices.Contains(cmd.Env, "PATH=/usr/bin:/bin") {
		t.Errorf("Env = %v, want the operational variables a program needs", cmd.Env)
	}
}

/*
A variable the worker does not have is not invented.

`TMPDIR=` is not the same as no TMPDIR: the first is a temporary directory that
does not exist, and a program handed one fails somewhere far from here. The
worker's environment is copied, never approximated.

Unset with os.Unsetenv rather than `t.Setenv(name, "")`, which sets it to
empty — the first version of this test did that and passed on the behaviour it
was written to forbid.
*/
func TestCommandFor_aVariableTheWorkerDoesNotHold_isNotPassedEmpty(t *testing.T) {
	t.Setenv("TMPDIR", "restored-by-cleanup")
	if err := os.Unsetenv("TMPDIR"); err != nil {
		t.Fatalf("unset: %v", err)
	}

	cmd, cleanup, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	t.Cleanup(cleanup)
	if slices.ContainsFunc(cmd.Env, func(v string) bool {
		return strings.HasPrefix(v, "TMPDIR=")
	}) {
		t.Errorf("Env = %v, want an unset variable left unset", cmd.Env)
	}
}

// accepted is a local server somebody has explicitly agreed to run.
func accepted() domain.MCPServer {
	return domain.MCPServer{
		Name: "local", Transport: "stdio", Command: "/bin/true",
		AcceptsLocalExecution: true,
	}
}

/*
A local server nobody accepted is not started.

Refused here as well as where it is written, because a row can arrive by
restore, by migration, or from a version of the console that did not ask. The
door checking a rule the runtime does not is a rule that holds until the first
time it matters.
*/
func TestCommandFor_aLocalServerNobodyAccepted_isNotStarted(t *testing.T) {
	server := accepted()
	server.AcceptsLocalExecution = false

	if _, _, err := commandFor(t.Context(), server, domain.MCPCredentials{}); err == nil {
		t.Fatal("no error; a program nobody agreed to would have been started")
	} else if !strings.Contains(err.Error(), "local") {
		t.Errorf("err = %v, want a sentence naming what was not accepted", err)
	}
}

/*
A local server's own variables reach it, and only its own.

The allowlist closed the hole and took the capability with it: before this,
inheritance was how a local server ever got a token. So it receives them
explicitly, from the vault, per server — and the worker's own secrets still do
not travel, which is the property the whole change exists for.
*/
func TestCommandFor_theServersOwnVariables_areGivenWithoutReopeningInheritance(t *testing.T) {
	t.Setenv("FUSEONE_MASTER_KEY", "must-not-travel")
	t.Setenv("PATH", "/usr/bin:/bin")

	cmd, cleanup, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{
		Env: map[string]string{"GITHUB_TOKEN": "ghp_configured"},
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	t.Cleanup(cleanup)

	if !slices.Contains(cmd.Env, "GITHUB_TOKEN=ghp_configured") {
		t.Errorf("Env = %v, want the configured variable", cmd.Env)
	}
	if !slices.Contains(cmd.Env, "PATH=/usr/bin:/bin") {
		t.Errorf("Env = %v, want the allowlist as well", cmd.Env)
	}
	if slices.ContainsFunc(cmd.Env, func(v string) bool {
		return strings.HasPrefix(v, "FUSEONE_MASTER_KEY=")
	}) {
		t.Error("configuring a variable reopened inheritance")
	}
}

// A server that names a variable the allowlist also carries means the one it
// named. Its own configuration is the more specific statement.
func TestCommandFor_aConfiguredVariable_winsOverTheOneCopiedThrough(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")

	cmd, cleanup, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{
		Env: map[string]string{"PATH": "/opt/tools/bin"},
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	t.Cleanup(cleanup)

	// Both are present and the configured one is last, which is what an
	// exec environment resolves to.
	last := ""
	for _, v := range cmd.Env {
		if strings.HasPrefix(v, "PATH=") {
			last = v
		}
	}
	if last != "PATH=/opt/tools/bin" {
		t.Errorf("PATH resolves to %q, want the configured one", last)
	}
}

func TestAuthenticatedClient_sendsBearerTokensToRemoteServers(t *testing.T) {
	t.Parallel()

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ghp_secret" {
			t.Errorf("Authorization = %q, want the bearer token", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(remote.Close)

	client, err := authenticatedClient("github", domain.MCPCredentials{Token: "ghp_secret"}, nil, nil)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	resp, err := client.Get(remote.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()
}

func TestAuthenticatedClient_refusesCloudMetadataAtDialTime(t *testing.T) {
	t.Parallel()

	client, err := authenticatedClient("metadata", domain.MCPCredentials{Token: "secret"}, nil, nil)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	_, err = client.Get("http://169.254.169.254/latest/meta-data")
	if !errors.Is(err, netguard.ErrBlockedAddress) {
		t.Fatalf("Get = %v, want metadata address refused", err)
	}
}

func TestAuthenticatedClient_doesNotShareTheDefaultTransport(t *testing.T) {
	t.Parallel()

	bearerClient, err := authenticatedClient("github", domain.MCPCredentials{Token: "ghp_secret"}, nil, nil)
	if err != nil {
		t.Fatalf("authenticatedClient bearer: %v", err)
	}
	bearerTransport, ok := bearerClient.Transport.(*credentialAuth)
	if !ok {
		t.Fatalf("bearer transport = %T, want *credentialAuth", bearerClient.Transport)
	}
	if bearerTransport.base == http.DefaultTransport {
		t.Fatal("bearer client shares http.DefaultTransport")
	}

	headerClient, err := authenticatedClient("newrelic", domain.MCPCredentials{
		Headers: map[string]string{"Api-Key": "nr_secret"},
	}, nil, nil)
	if err != nil {
		t.Fatalf("authenticatedClient headers: %v", err)
	}
	headerTransport, ok := headerClient.Transport.(*credentialAuth)
	if !ok {
		t.Fatalf("header transport = %T, want *credentialAuth", headerClient.Transport)
	}
	if headerTransport.base == http.DefaultTransport {
		t.Fatal("header client shares http.DefaultTransport")
	}

	oauthClient, err := authenticatedClient("google", domain.MCPCredentials{
		OAuth: &domain.MCPOAuthGrant{AccessToken: "oauth_access"},
	}, nil, nil)
	if err != nil {
		t.Fatalf("authenticatedClient oauth: %v", err)
	}
	oauthTransport, ok := oauthClient.Transport.(*credentialAuth)
	if !ok {
		t.Fatalf("oauth transport = %T, want *credentialAuth", oauthClient.Transport)
	}
	if oauthTransport.base == http.DefaultTransport {
		t.Fatal("oauth client shares http.DefaultTransport")
	}
	grantTransport := newOAuthTransport("google", "", domain.MCPOAuthGrant{}, nil, baseHTTPTransport())
	if grantTransport.refresh.Transport == http.DefaultTransport {
		t.Fatal("oauth refresh client shares http.DefaultTransport")
	}
}

func TestAuthenticatedClient_sendsConfiguredHeaderCredentialsToRemoteServers(t *testing.T) {
	t.Parallel()

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Api-Key"); got != "nr_secret" {
			t.Errorf("Api-Key = %q, want the configured header", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want no invented bearer header", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(remote.Close)

	client, err := authenticatedClient("newrelic", domain.MCPCredentials{
		Headers: map[string]string{"Api-Key": "nr_secret"},
	}, nil, nil)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	resp, err := client.Get(remote.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()
}

func TestAuthenticatedClient_oauthWinsOverABearerLeftBehind(t *testing.T) {
	t.Parallel()

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer oauth_access" {
			t.Errorf("Authorization = %q, want the oauth access token", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(remote.Close)

	client, err := authenticatedClient("google", domain.MCPCredentials{
		Token: "ghp_stale",
		OAuth: &domain.MCPOAuthGrant{
			AccessToken: "oauth_access",
		},
	}, nil, nil)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	resp, err := client.Get(remote.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()
}

func TestAuthenticatedClient_refreshesAnExpiredOAuthGrant(t *testing.T) {
	t.Parallel()

	var refreshed bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			refreshed = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if r.Form.Get("grant_type") != "refresh_token" ||
				r.Form.Get("refresh_token") != "refresh" ||
				r.Form.Get("client_id") != "client" ||
				r.Form.Get("client_secret") != "secret" {
				t.Fatalf("refresh form = %v, want the stored grant", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "fresh",
				"refresh_token": "new-refresh",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"scope":         "read write",
			})
		case "/resource":
			if got := r.Header.Get("Authorization"); got != "Bearer fresh" {
				t.Errorf("Authorization = %q, want the refreshed token", got)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(remote.Close)

	store := &recordingOAuthStore{}
	client, err := authenticatedClient("google", domain.MCPCredentials{
		OAuth: &domain.MCPOAuthGrant{
			AccessToken:   "expired",
			RefreshToken:  "refresh",
			TokenURL:      remote.URL + "/token",
			ClientID:      "client",
			ClientSecret:  "secret",
			ExpiresAtUnix: time.Now().Add(-time.Minute).Unix(),
		},
	}, store, nil)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	resp, err := client.Get(remote.URL + "/resource")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()
	if !refreshed {
		t.Fatal("expired oauth grant was used without refresh")
	}
	if store.name != "google" ||
		store.owner != "" ||
		store.was.RefreshToken != "refresh" ||
		store.next.AccessToken != "fresh" ||
		store.next.RefreshToken != "new-refresh" ||
		store.next.Scopes[0] != "read" {
		t.Fatalf("persisted = %s %+v -> %+v, want the refreshed grant", store.name, store.was, store.next)
	}
}

func TestAuthenticatedClient_refusesAnExpiredOAuthGrantItCannotRefresh(t *testing.T) {
	t.Parallel()

	remote := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request reached the server with an expired unrefreshable grant")
	}))
	t.Cleanup(remote.Close)

	client, err := authenticatedClient("google", domain.MCPCredentials{
		OAuth: &domain.MCPOAuthGrant{
			AccessToken:   "expired",
			ExpiresAtUnix: time.Now().Add(-time.Minute).Unix(),
		},
	}, nil, nil)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	if _, err := client.Get(remote.URL); err == nil {
		t.Fatal("Get succeeded with an expired grant that cannot refresh")
	} else if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("Get error = %v, want the expired grant named", err)
	}
}

func TestAuthenticatedClient_aRefreshThatCannotBePersistedIsNotSent(t *testing.T) {
	t.Parallel()

	store := &recordingOAuthStore{err: errors.New("vault unavailable")}
	var resourceCalled bool
	tokenCalls := 0
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "fresh",
				"refresh_token": "new-refresh",
				"expires_in":    3600,
			})
		case "/resource":
			resourceCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(remote.Close)

	client, err := authenticatedClient("google", domain.MCPCredentials{
		OAuth: &domain.MCPOAuthGrant{
			AccessToken:   "expired",
			RefreshToken:  "refresh",
			TokenURL:      remote.URL + "/token",
			ExpiresAtUnix: time.Now().Add(-time.Minute).Unix(),
		},
	}, store, nil)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	if _, err := client.Get(remote.URL + "/resource"); err == nil {
		t.Fatal("Get succeeded even though the refreshed grant was not persisted")
	} else if !strings.Contains(err.Error(), "persist refreshed oauth grant") {
		t.Fatalf("Get error = %v, want persistence failure named", err)
	}
	if resourceCalled {
		t.Fatal("request reached the MCP server before the refreshed grant was persisted")
	}
	if store.next.RefreshToken != "new-refresh" {
		t.Fatalf("persisted = %+v, want the rotated refresh token attempted", store.next)
	}

	store.err = nil
	resourceCalled = false
	resp, err := client.Get(remote.URL + "/resource")
	if err != nil {
		t.Fatalf("second Get after the vault recovered: %v", err)
	}
	_ = resp.Body.Close()
	if !resourceCalled {
		t.Fatal("request did not reach the MCP server after the pending grant was persisted")
	}
	if tokenCalls != 1 {
		t.Fatalf("token endpoint called %d times, want the pending grant saved before another refresh", tokenCalls)
	}
}

func TestAuthenticatedClient_prefersPersonalCredentialsForToolCalls(t *testing.T) {
	t.Parallel()

	var seen []string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(remote.Close)

	client, err := authenticatedClient("github",
		domain.MCPCredentials{Token: "shared"},
		nil,
		personalCreds{"usr_ana": domain.MCPCredentials{Token: "ana"}},
	)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	req, err := http.NewRequestWithContext(
		tools.WithCaller(t.Context(), "usr_ana"), http.MethodGet, remote.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("personal request: %v", err)
	}
	_ = resp.Body.Close()
	resp, err = client.Get(remote.URL)
	if err != nil {
		t.Fatalf("shared request: %v", err)
	}
	_ = resp.Body.Close()

	if len(seen) != 2 ||
		seen[0] != "Bearer ana" ||
		seen[1] != "Bearer shared" {
		t.Fatalf("Authorization = %v, want personal then shared", seen)
	}
}

func TestAuthenticatedClient_aUserOnlyServerUsesTheSharedCredentialForDiscovery(t *testing.T) {
	t.Parallel()

	var seen string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(remote.Close)

	client, err := authenticatedClientWithPolicy("google",
		domain.MCPCredentials{Token: "shared"},
		nil, personalCreds{},
		credentialPolicy{requirePersonal: true},
	)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	resp, err := client.Get(remote.URL)
	if err != nil {
		t.Fatalf("discovery request: %v", err)
	}
	_ = resp.Body.Close()

	if seen != "Bearer shared" {
		t.Fatalf("Authorization = %q, want the shared discovery credential", seen)
	}
}

func TestAuthenticatedClient_aUserOnlyServerDoesNotUseTheSharedCredentialForACronCall(t *testing.T) {
	t.Parallel()

	remote := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("a cron tool call reached the server with the shared credential")
	}))
	t.Cleanup(remote.Close)

	client, err := authenticatedClientWithPolicy("google",
		domain.MCPCredentials{Token: "shared"},
		nil, personalCreds{},
		credentialPolicy{requirePersonal: true},
	)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	req, err := http.NewRequestWithContext(
		tools.WithInvocation(t.Context()), http.MethodGet, remote.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := client.Do(req); err == nil {
		t.Fatal("cron call succeeded by falling back to the shared credential")
	} else if !strings.Contains(err.Error(), "requires a personal credential") {
		t.Fatalf("Get error = %v, want the missing person named", err)
	}
}

func TestAuthenticatedClient_aUserOnlyServerStillBlocksToolCallsWithoutAnyCredentialStore(t *testing.T) {
	t.Parallel()

	remote := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("a user-only call reached the server without a personal credential store")
	}))
	t.Cleanup(remote.Close)

	client, err := authenticatedClientWithPolicy("google",
		domain.MCPCredentials{},
		nil, nil,
		credentialPolicy{requirePersonal: true},
	)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	if client == nil {
		t.Fatal("client is nil; the user-only invocation policy would never run")
	}
	req, err := http.NewRequestWithContext(
		tools.WithInvocation(t.Context()), http.MethodGet, remote.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := client.Do(req); err == nil {
		t.Fatal("call succeeded without a personal credential store")
	} else if !strings.Contains(err.Error(), "requires a personal credential") {
		t.Fatalf("Get error = %v, want the missing person named", err)
	}
}

func TestAuthenticatedClient_aUserOnlyServerDoesNotUseTheSharedCredentialWhenThePersonHasNone(t *testing.T) {
	t.Parallel()

	remote := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("a user-only call reached the server with the shared credential")
	}))
	t.Cleanup(remote.Close)

	client, err := authenticatedClientWithPolicy("google",
		domain.MCPCredentials{Token: "shared"},
		nil, personalCreds{},
		credentialPolicy{requirePersonal: true},
	)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	ctx := tools.WithInvocation(tools.WithCaller(t.Context(), "usr_ana"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remote.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := client.Do(req); err == nil {
		t.Fatal("call succeeded by falling back to the shared credential")
	} else if !strings.Contains(err.Error(), "installation credential for discovery") ||
		!strings.Contains(err.Error(), "usr_ana") {
		t.Fatalf("Get error = %v, want Ana's missing personal credential named beside the shared discovery credential", err)
	}
}

func TestAuthenticatedClient_readsPersonalCredentialsOnEachCall(t *testing.T) {
	t.Parallel()

	var seen []string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(remote.Close)

	personal := rotatingPersonal{creds: domain.MCPCredentials{Token: "first"}}
	client, err := authenticatedClient("github", domain.MCPCredentials{}, nil, &personal)
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	for _, token := range []string{"first", "second"} {
		personal.creds = domain.MCPCredentials{Token: token}
		req, err := http.NewRequestWithContext(
			tools.WithCaller(t.Context(), "usr_ana"), http.MethodGet, remote.URL, nil)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request with %s: %v", token, err)
		}
		_ = resp.Body.Close()
	}

	if len(seen) != 2 || seen[0] != "Bearer first" || seen[1] != "Bearer second" {
		t.Fatalf("Authorization = %v, want the latest personal token each time", seen)
	}
}

func TestAuthenticatedClient_persistsPersonalOAuthRefreshForThatUser(t *testing.T) {
	t.Parallel()

	store := &recordingOAuthStore{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "fresh",
				"refresh_token": "new-refresh",
				"expires_in":    3600,
			})
		case "/resource":
			if got := r.Header.Get("Authorization"); got != "Bearer fresh" {
				t.Errorf("Authorization = %q, want the refreshed personal token", got)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(remote.Close)

	grant := domain.MCPOAuthGrant{
		AccessToken:   "expired",
		RefreshToken:  "refresh",
		TokenURL:      remote.URL + "/token",
		ExpiresAtUnix: time.Now().Add(-time.Minute).Unix(),
	}
	client, err := authenticatedClient("google", domain.MCPCredentials{}, store,
		personalCreds{"usr_ana": domain.MCPCredentials{OAuth: &grant}})
	if err != nil {
		t.Fatalf("authenticatedClient: %v", err)
	}
	req, err := http.NewRequestWithContext(
		tools.WithCaller(t.Context(), "usr_ana"), http.MethodGet, remote.URL+"/resource", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()

	if store.name != "google" ||
		store.owner != "usr_ana" ||
		store.was.RefreshToken != "refresh" ||
		store.next.RefreshToken != "new-refresh" {
		t.Fatalf("persisted = %s/%s %+v -> %+v, want Ana's grant refreshed",
			store.name, store.owner, store.was, store.next)
	}
}

type recordingOAuthStore struct {
	name  string
	owner domain.UserID
	was   domain.MCPOAuthGrant
	next  domain.MCPOAuthGrant
	err   error
}

func (s *recordingOAuthStore) RefreshMCPServerOAuth(
	_ context.Context, name string, owner domain.UserID, was, next domain.MCPOAuthGrant,
) error {
	s.name, s.owner, s.was, s.next = name, owner, was, next
	return s.err
}

type personalCreds map[domain.UserID]domain.MCPCredentials

func (p personalCreds) MCPPersonalCredential(
	_ context.Context, _ string, principal domain.UserID,
) (domain.MCPCredentials, bool, error) {
	creds, ok := p[principal]
	return creds, ok, nil
}

type rotatingPersonal struct {
	creds domain.MCPCredentials
	err   error
}

func (p *rotatingPersonal) MCPPersonalCredential(
	context.Context, string, domain.UserID,
) (domain.MCPCredentials, bool, error) {
	return p.creds, !p.creds.Empty(), p.err
}

/*
A managed config file is content, not a path typed into args.

The platform owns the file it creates: it writes it in a private temporary
directory, hands the path to the local process by environment, and removes it
when the MCP session is closed. That keeps the operator from having to place a
secret file on the worker's filesystem by hand.
*/
func TestCommandFor_aConfigFile_isMaterializedAndNamedByEnvironment(t *testing.T) {
	server := accepted()
	cmd, cleanup, err := commandFor(t.Context(), server, domain.MCPCredentials{
		ConfigFile: "dsn: postgres://agent:secret@db/app\n",
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}

	path := envValue(cmd.Env, domain.DefaultMCPConfigFileEnv)
	if path == "" {
		t.Fatalf("Env = %v, want the managed config path", cmd.Env)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read materialized config: %v", err)
	}
	if string(body) != "dsn: postgres://agent:secret@db/app\n" {
		t.Errorf("config body = %q", body)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat materialized config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %v, want 0600", got)
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("after cleanup stat = %v, want file gone", err)
	}
}

func TestCommandFor_aConfigFile_canUseACustomEnvironmentName(t *testing.T) {
	name := "TOOLBOX_CONFIG"
	server := accepted()
	server.ConfigFileEnv = &name

	cmd, cleanup, err := commandFor(t.Context(), server, domain.MCPCredentials{
		ConfigFile: "sources: []\n",
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	t.Cleanup(cleanup)

	if envValue(cmd.Env, "TOOLBOX_CONFIG") == "" {
		t.Fatalf("Env = %v, want TOOLBOX_CONFIG to name the materialized file", cmd.Env)
	}
}

func TestCommandFor_aManagedConfigPathWinsOverAHandWrittenVariable(t *testing.T) {
	cmd, cleanup, err := commandFor(t.Context(), accepted(), domain.MCPCredentials{
		Env:        map[string]string{domain.DefaultMCPConfigFileEnv: "/tmp/hand-written.yaml"},
		ConfigFile: "managed: true\n",
	})
	if err != nil {
		t.Fatalf("commandFor: %v", err)
	}
	t.Cleanup(cleanup)

	if got := envValue(cmd.Env, domain.DefaultMCPConfigFileEnv); got == "/tmp/hand-written.yaml" || got == "" {
		t.Fatalf("config env resolves to %q, want the managed path to win", got)
	}
}

func envValue(env []string, name string) string {
	prefix := name + "="
	out := ""
	for _, one := range env {
		if strings.HasPrefix(one, prefix) {
			out = strings.TrimPrefix(one, prefix)
		}
	}
	return out
}

/*
A rotated credential takes effect on the next pass, not the next deploy.

The reconciler keeps a session while the fingerprint holds, and the fingerprint
saw the address and not the secret — same command, same URL, same flags — so a
replaced token left the old session in use until a restart or an unrelated
edit. A credential is rotated most urgently when it has leaked, which is
exactly when "at the next deploy" is the wrong answer.

The timestamp stands in for the secret. Reading every server's credential on
every pass would unseal the vault on a timer to answer what the row answers.
*/
func TestFingerprint_aServerWrittenAgain_isNotTheOneInHand(t *testing.T) {
	t.Parallel()

	before := domain.MCPServer{
		Name: "github", Transport: domain.TransportHTTP,
		URL: "https://api.example.com/mcp", Enabled: true,
		UpdatedAt: time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC),
	}
	rotated := before
	rotated.UpdatedAt = before.UpdatedAt.Add(time.Minute)

	if fingerprint(before) == fingerprint(rotated) {
		t.Error("unchanged; the worker would go on using the credential that was replaced")
	}
}

// And a pass that changed nothing keeps the session. Reconnecting every server
// on every sweep would make a tool call fail whenever a pass landed on it.
func TestFingerprint_aServerNobodyTouched_keepsItsSession(t *testing.T) {
	t.Parallel()

	server := domain.MCPServer{
		Name: "github", Transport: domain.TransportHTTP,
		URL: "https://api.example.com/mcp", Enabled: true,
		UpdatedAt: time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC),
	}
	if fingerprint(server) != fingerprint(server) {
		t.Error("a server reconnects on every pass")
	}
}

func TestFingerprint_aRateLimitChangeReconnectsTheServer(t *testing.T) {
	t.Parallel()

	before := domain.MCPServer{
		Name: "github", Transport: domain.TransportHTTP,
		URL: "https://api.example.com/mcp", Enabled: true,
		UpdatedAt: time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC),
		RateLimit: &domain.MCPRateLimit{RatePerSecond: 1, Burst: 2},
	}
	after := before
	after.RateLimit = &domain.MCPRateLimit{RatePerSecond: 0.5, Burst: 2}

	if fingerprint(before) == fingerprint(after) {
		t.Error("unchanged; the worker would keep the old MCP rate limit")
	}
}

func TestFingerprint_aResultCacheChangeReconnectsTheServer(t *testing.T) {
	t.Parallel()

	before := domain.MCPServer{
		Name: "github", Transport: domain.TransportHTTP,
		URL: "https://api.example.com/mcp", Enabled: true,
		UpdatedAt: time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC),
		Cache:     &domain.MCPResultCache{TTLSeconds: 30, MaxEntries: 256},
	}
	after := before
	after.Cache = &domain.MCPResultCache{TTLSeconds: 5, MaxEntries: 256}

	if fingerprint(before) == fingerprint(after) {
		t.Error("unchanged; the worker would keep the old MCP result cache")
	}
}

func TestTransportForHTTP_doesNotOpenAStandaloneSSEStream(t *testing.T) {
	t.Parallel()

	transport, cleanup, err := transportFor(t.Context(), domain.MCPServer{
		Name: "grafana", Transport: domain.TransportHTTP, URL: "https://grafana.example/mcp",
	}, domain.MCPCredentials{}, nil, nil, credentialPolicy{})
	if err != nil {
		t.Fatalf("transportFor: %v", err)
	}
	t.Cleanup(cleanup)

	streamable, ok := transport.(*mcp.StreamableClientTransport)
	if !ok {
		t.Fatalf("transport = %T, want StreamableClientTransport", transport)
	}
	if !streamable.DisableStandaloneSSE {
		t.Fatal("the worker would keep an optional idle SSE stream and close the MCP client when a proxy drops it")
	}
}

func TestConnectServer_legacyHTTPProtocolDoesNotSendModernDiscover(t *testing.T) {
	t.Parallel()

	var sawDiscover atomic.Bool
	var sawToolsList atomic.Bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ProtocolVersion string `json:"protocolVersion"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch req.Method {
		case "server/discover":
			sawDiscover.Store(true)
			http.Error(w, "Bad Request: Unsupported protocol version: 2026-07-28", http.StatusBadRequest)
		case "initialize":
			if got := req.Params.ProtocolVersion; got != "2025-11-25" {
				t.Errorf("initialize protocol = %q, want legacy 2025-11-25", got)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "outline-legacy")
			respondRPC(t, w, req.ID, map[string]any{
				"protocolVersion": "2025-11-25",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "outline", "version": "test"},
			})
		case "notifications/initialized":
			if got := r.Header.Get("Mcp-Protocol-Version"); got == "2026-07-28" {
				t.Errorf("initialized used the modern MCP protocol header")
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			sawToolsList.Store(true)
			if got := r.Header.Get("Mcp-Protocol-Version"); got == "2026-07-28" {
				t.Errorf("tools/list used the modern MCP protocol header")
			}
			respondRPC(t, w, req.ID, map[string]any{
				"tools": []map[string]any{{
					"name":        "search",
					"description": "Search documents.",
					"inputSchema": map[string]any{"type": "object"},
				}},
			})
		default:
			t.Errorf("unexpected MCP method %q", req.Method)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(remote.Close)

	catalog := tools.NewCatalog(engine.NewMemoryContent())
	err := connectServer(t.Context(), catalog, domain.MCPServer{
		Name: "outline", Transport: domain.TransportHTTP, URL: remote.URL,
		ProtocolMode: domain.MCPProtocolLegacy, Enabled: true,
	}, domain.MCPCredentials{}, nil, nil, credentialPolicy{})
	if err != nil {
		t.Fatalf("connectServer: %v", err)
	}
	t.Cleanup(func() { _ = catalog.RemoveServer("outline") })

	if sawDiscover.Load() {
		t.Fatal("the legacy compatibility mode still sent server/discover upstream")
	}
	if !sawToolsList.Load() {
		t.Fatal("the connection did not get far enough to list tools")
	}
	if _, known := catalog.Lookup("outline.search"); !known {
		t.Fatal("the legacy MCP server's tool was not imported")
	}
}

func respondRPC(t *testing.T, w http.ResponseWriter, id json.RawMessage, result any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	if err := json.NewEncoder(w).Encode(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}); err != nil {
		t.Fatalf("write JSON-RPC response: %v", err)
	}
}

func TestReconciler_aProbeRequestReconnectsThroughTheWorkerPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "crm", Transport: domain.TransportHTTP, URL: "https://tools.example.com/mcp",
		Enabled: true, UpdatedAt: time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC),
	}
	configured := &probeServers{servers: []domain.MCPServer{server}, probes: []string{"crm"}}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	old := &testSession{tools: []*mcp.Tool{{Name: "old"}}}
	if err := catalog.AddServer(ctx, "crm", old, nil); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	health := recordingHealth{}
	publisher := &recordingPublisher{}
	r := newReconciler(catalog, configured, health).publishingTo(publisher)
	r.connected["crm"] = fingerprint(server)
	r.connectTo = func(
		ctx context.Context, catalog *tools.Catalog, server domain.MCPServer,
		_ domain.MCPCredentials, _ OAuthGrantStore, _ MCPUserCredentialStore, _ credentialPolicy,
	) error {
		return catalog.AddServer(ctx, server.Name, &testSession{tools: []*mcp.Tool{{
			Name:        "lookup",
			Description: "Look a customer up.",
			InputSchema: map[string]any{"type": "object"},
		}}}, server.Surface)
	}

	r.reconcile(ctx)

	if !old.closed {
		t.Error("the successful probe did not replace the old session")
	}
	if _, known := catalog.Effect("crm.old"); known {
		t.Error("the old tool survived the probe replacement")
	}
	if _, known := catalog.Effect("crm.lookup"); !known {
		t.Error("the probed tool was not published into the runtime catalogue")
	}
	if len(configured.probes) != 0 {
		t.Fatalf("probe was not consumed: %v", configured.probes)
	}
	if len(publisher.entries) == 0 {
		t.Fatal("the changed catalogue was not published")
	}
	if seen := health["crm"]; !seen.Reachable || seen.ToolCount != 1 {
		t.Fatalf("health = %+v, want the successful probe observation", seen)
	}
}

func TestReconciler_aFailedProbeKeepsTheCurrentSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "crm", Transport: domain.TransportHTTP, URL: "https://tools.example.com/mcp",
		Enabled: true, UpdatedAt: time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC),
	}
	configured := &probeServers{servers: []domain.MCPServer{server}, probes: []string{"crm"}}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	old := &testSession{tools: []*mcp.Tool{{Name: "lookup"}}}
	if err := catalog.AddServer(ctx, "crm", old, nil); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	health := recordingHealth{}
	publisher := &recordingPublisher{}
	r := newReconciler(catalog, configured, health).publishingTo(publisher)
	r.connected["crm"] = fingerprint(server)
	r.connectTo = func(
		context.Context, *tools.Catalog, domain.MCPServer,
		domain.MCPCredentials, OAuthGrantStore, MCPUserCredentialStore, credentialPolicy,
	) error {
		return errors.New("dial timeout")
	}

	r.reconcile(ctx)

	if old.closed {
		t.Error("a failed probe closed the live session")
	}
	if _, known := catalog.Effect("crm.lookup"); !known {
		t.Error("the current tool disappeared after a failed probe")
	}
	if len(publisher.entries) != 0 {
		t.Fatalf("published after a failed probe: %+v", publisher.entries)
	}
	if seen := health["crm"]; seen.Reachable || !strings.Contains(seen.Detail, "dial timeout") {
		t.Fatalf("health = %+v, want the failed probe observation", seen)
	}
}

func TestReconciler_aReconnectedServerGetsItsRecordedRulingsBeforeItCanRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "crm", Transport: domain.TransportHTTP, URL: "https://tools.example.com/mcp",
		Enabled: true, UpdatedAt: time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC),
	}
	configured := &probeServers{servers: []domain.MCPServer{server}, probes: []string{"crm"}}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	old := &testSession{tools: []*mcp.Tool{{Name: "old"}}}
	if err := catalog.AddServer(ctx, "crm", old, nil); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	rulings := &recordingRulings{}
	r := newReconciler(catalog, configured, recordingHealth{}).classifyingWith(rulings)
	r.connected["crm"] = fingerprint(server)
	r.connectTo = func(
		ctx context.Context, catalog *tools.Catalog, server domain.MCPServer,
		_ domain.MCPCredentials, _ OAuthGrantStore, _ MCPUserCredentialStore, _ credentialPolicy,
	) error {
		if err := catalog.AddServer(ctx, server.Name, &testSession{tools: []*mcp.Tool{{
			Name:        "fetch",
			Description: "Fetch a document.",
			InputSchema: map[string]any{"type": "object"},
		}}}, server.Surface); err != nil {
			return err
		}
		for _, entry := range catalog.List() {
			if entry.ID == "crm.fetch" {
				rulings.rulings = []domain.ToolClassification{{
					Tool: entry.ID, Effect: domain.EffectRead, Digest: entry.Digest,
				}}
				return nil
			}
		}
		return errors.New("crm.fetch was not imported")
	}

	r.reconcile(ctx)

	if rulings.calls == 0 {
		t.Fatal("the reconciler did not refresh classifications after discovery")
	}
	if effect, ok := catalog.Effect("crm.fetch"); !ok || effect != domain.EffectRead {
		t.Fatalf("effect = %v (%v), want the recorded ruling applied before a tool call", effect, ok)
	}
}

func TestReconciler_aUserOnlyRecipeCarriesTheCredentialPolicyToTheConnection(t *testing.T) {
	t.Parallel()

	server := domain.MCPServer{
		Name: "google-sheets", Transport: domain.TransportHTTP,
		URL: "https://sheets.example.com/mcp", Enabled: true,
	}
	configured := &probeServers{servers: []domain.MCPServer{server}}
	r := newReconciler(
		tools.NewCatalog(engine.NewMemoryContent()), configured, recordingHealth{},
	).withCredentialPolicy(personalRequirement{"google-sheets": true})

	seen := false
	r.connectTo = func(
		ctx context.Context, catalog *tools.Catalog, server domain.MCPServer,
		_ domain.MCPCredentials, _ OAuthGrantStore, _ MCPUserCredentialStore, policy credentialPolicy,
	) error {
		seen = policy.requirePersonal
		return catalog.AddServer(ctx, server.Name, &testSession{tools: []*mcp.Tool{{Name: "get_values"}}}, server.Surface)
	}

	r.reconcile(t.Context())

	if !seen {
		t.Fatal("connected a user-only recipe without requiring a personal credential for calls")
	}
}

func TestReconciler_aKnownLegacyRecipeCarriesTheProtocolToTheConnection(t *testing.T) {
	t.Parallel()

	server := domain.MCPServer{
		Name: "outline", Transport: domain.TransportHTTP,
		URL: "https://outline.example.com/mcp", Enabled: true,
	}
	configured := &probeServers{servers: []domain.MCPServer{server}}
	r := newReconciler(
		tools.NewCatalog(engine.NewMemoryContent()), configured, recordingHealth{},
	).withProtocolPolicy(protocolRequirement{"outline": domain.MCPProtocolLegacy})

	seen := ""
	r.connectTo = func(
		ctx context.Context, catalog *tools.Catalog, server domain.MCPServer,
		_ domain.MCPCredentials, _ OAuthGrantStore, _ MCPUserCredentialStore, _ credentialPolicy,
	) error {
		seen = server.MCPProtocolModeOf()
		return catalog.AddServer(ctx, server.Name, &testSession{tools: []*mcp.Tool{{Name: "search"}}}, server.Surface)
	}

	r.reconcile(t.Context())

	if seen != domain.MCPProtocolLegacy {
		t.Fatalf("protocol mode = %q, want legacy from the known recipe", seen)
	}
}

type probeServers struct {
	servers []domain.MCPServer
	probes  []string
}

func (p *probeServers) MCPServers(context.Context) ([]domain.MCPServer, error) {
	return append([]domain.MCPServer(nil), p.servers...), nil
}

func (p *probeServers) MCPCredentials(context.Context, string) (domain.MCPCredentials, error) {
	return domain.MCPCredentials{}, nil
}

func (p *probeServers) ClaimMCPProbes(_ context.Context, limit int) ([]string, error) {
	if limit > len(p.probes) {
		limit = len(p.probes)
	}
	out := append([]string(nil), p.probes[:limit]...)
	p.probes = p.probes[limit:]
	return out, nil
}

type personalRequirement map[string]bool

func (p personalRequirement) RequiresPersonalCredential(server string) bool {
	return p[server]
}

type protocolRequirement map[string]string

func (p protocolRequirement) MCPProtocolMode(server string) string {
	return p[server]
}

type recordingRulings struct {
	calls   int
	rulings []domain.ToolClassification
}

func (r *recordingRulings) List(context.Context, domain.Scope) ([]domain.ToolClassification, error) {
	r.calls++
	return append([]domain.ToolClassification(nil), r.rulings...), nil
}

type testSession struct {
	tools  []*mcp.Tool
	closed bool
}

func (s *testSession) ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	return &mcp.ListToolsResult{Tools: s.tools}, nil
}

func (s *testSession) CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return nil, errors.New("not called")
}

func (s *testSession) Close() error {
	s.closed = true
	return nil
}

type recordingHealth map[string]domain.IntegrationHealth

func (r recordingHealth) Record(_ context.Context, h domain.IntegrationHealth) error {
	r[h.Name] = h
	return nil
}

type recordingPublisher struct {
	entries []domain.ToolEntry
}

func (r *recordingPublisher) Publish(_ context.Context, entries []domain.ToolEntry) error {
	r.entries = append([]domain.ToolEntry(nil), entries...)
	return nil
}
