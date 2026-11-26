package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

/*
A server that knows who is calling by the token it saw at initialize answers
each person as themselves only when each person's session was opened with
their own token. Before, one session for everybody was initialized with the
shared credential, and a person's token only rode on the requests after it.
*/

// identityServer requires a token on every request and remembers, for each
// session, the token that initialized it and those its tool calls carried.
type identityServer struct {
	mu       sync.Mutex
	openedBy map[string]string   // session id → Authorization at initialize
	calls    map[string][]string // session id → Authorization of each tools/call
}

func newIdentityServer(t *testing.T) (*identityServer, string) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "dpg", Version: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "whoami"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	ids := &identityServer{openedBy: map[string]string{}, calls: map[string][]string{}}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		// Each request is recorded before the client can see its answer: the
		// client may have its response, and the call return, while the
		// handler is still running.
		if bytes.Contains(body, []byte(`"method":"tools/call"`)) {
			ids.called(r.Header.Get("Mcp-Session-Id"), auth)
		}
		if bytes.Contains(body, []byte(`"method":"initialize"`)) {
			w = &onHeader{ResponseWriter: w, wrote: func(h http.Header) { ids.opened(h.Get("Mcp-Session-Id"), auth) }}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(remote.Close)
	return ids, remote.URL
}

func (s *identityServer) opened(session, auth string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openedBy[session] = auth
}

func (s *identityServer) called(session, auth string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[session] = append(s.calls[session], auth)
}

// onHeader tells wrote the header of the response as it is sent.
type onHeader struct {
	http.ResponseWriter
	wrote func(http.Header)
	once  sync.Once
}

func (w *onHeader) WriteHeader(code int) {
	w.once.Do(func() { w.wrote(w.Header()) })
	w.ResponseWriter.WriteHeader(code)
}

func (w *onHeader) Write(b []byte) (int, error) {
	w.once.Do(func() { w.wrote(w.Header()) })
	return w.ResponseWriter.Write(b)
}

func (w *onHeader) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// callsBy is every tools/call, as "token that opened the session → token the
// call carried".
func (s *identityServer) callsBy() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for session, auths := range s.calls {
		for _, auth := range auths {
			out = append(out, s.openedBy[session]+" → "+auth)
		}
	}
	slices.Sort(out)
	return out
}

func (s *identityServer) initializedWith() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.openedBy))
	for _, auth := range s.openedBy {
		out = append(out, auth)
	}
	slices.Sort(out)
	return out
}

func connectPersonal(t *testing.T, url string) *tools.Catalog {
	t.Helper()
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	server := domain.MCPServer{Name: "dpg", Transport: domain.TransportHTTP, URL: url, Enabled: true}
	if err := connectServer(t.Context(), catalog, server, domain.MCPCredentials{Token: "shared"}, nil,
		personalCreds{"usr_ana": {Token: "ana"}, "usr_bia": {Token: "bia"}},
		credentialPolicy{requirePersonal: true}, nil); err != nil {
		t.Fatalf("connectServer: %v", err)
	}
	return catalog
}

func TestConnectServer_aServerTakingPersonalCredentials_opensEachPersonsSessionWithTheirToken(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	catalog := connectPersonal(t, url)

	for _, who := range []domain.UserID{"usr_ana", "usr_bia", "usr_ana"} {
		res, err := catalog.Invoke(t.Context(), engine.Call{RunID: "run-1", Seq: 1, Tool: "dpg.whoami", OnBehalfOf: who})
		if err != nil || res.Failed {
			t.Fatalf("call as %s = %+v, %v", who, res, err)
		}
	}
	want := []string{"Bearer ana → Bearer ana", "Bearer ana → Bearer ana", "Bearer bia → Bearer bia"}
	if got := ids.callsBy(); !slices.Equal(got, want) {
		t.Errorf("calls = %v, want each person's calls on a session their own token opened", got)
	}
}

// Someone with no credential of their own gets no session: the shared
// credential opens only the discovery session, never one on a person's behalf.
func TestConnectServer_aPersonWithNoCredential_getsNoSessionOpenedWithTheSharedOne(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	catalog := connectPersonal(t, url)

	res, err := catalog.Invoke(t.Context(), engine.Call{RunID: "run-1", Seq: 1, Tool: "dpg.whoami", OnBehalfOf: "usr_cai"})
	if err == nil || !res.Failed {
		t.Fatalf("call as Cai = %+v, %v; want it refused", res, err)
	}
	assertFailureCode(t, err, tools.CodeMCPPersonalCredentialMissing)
	if got := ids.initializedWith(); !slices.Equal(got, []string{"Bearer shared"}) {
		t.Errorf("sessions initialized with %v, want the discovery session alone", got)
	}
}

// The open is the person's whatever context it is handed: discovery on a
// person's behalf will open from one that names nobody.
func openAs(t *testing.T, url string, who domain.UserID) (tools.Session, error) {
	t.Helper()
	server := domain.MCPServer{Name: "dpg", Transport: domain.TransportHTTP, URL: url, Enabled: true}
	open := personalOpener(server, domain.MCPCredentials{Token: "shared"}, nil,
		personalCreds{"usr_ana": {Token: "ana"}}, credentialPolicy{requirePersonal: true})
	return open(context.Background(), who)
}

func TestPersonalOpener_fromAContextNamingNobody_opensAndCallsAsThePerson(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	session, err := openAs(t, url, "usr_ana")
	if err != nil {
		t.Fatalf("open as Ana: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "whoami"}); err != nil {
		t.Fatalf("call: %v", err)
	}
	if got := ids.callsBy(); !slices.Equal(got, []string{"Bearer ana → Bearer ana"}) {
		t.Errorf("calls = %v, want Ana's session opened and called with her token", got)
	}
}

func TestPersonalOpener_fromAContextNamingNobody_refusesSomeoneWithNoCredential(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	if session, err := openAs(t, url, "usr_cai"); err == nil {
		_ = session.Close()
		t.Fatal("opened a session for Cai, who has no credential")
	}
	if got := ids.initializedWith(); len(got) != 0 {
		t.Errorf("sessions initialized with %v, want none", got)
	}
}

// A person who changes their credential speaks as the new one from the next
// call: a server that fixed the old at initialize must not keep answering it.
func TestConnectServer_aPersonWhoChangesTheirCredential_isOpenedAnewWithIt(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	creds := personalCreds{"usr_ana": {Token: "first"}}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	server := domain.MCPServer{Name: "dpg", Transport: domain.TransportHTTP, URL: url, Enabled: true}
	if err := connectServer(t.Context(), catalog, server, domain.MCPCredentials{Token: "shared"}, nil,
		creds, credentialPolicy{requirePersonal: true}, nil); err != nil {
		t.Fatalf("connectServer: %v", err)
	}
	call := func() {
		t.Helper()
		if res, err := catalog.Invoke(t.Context(), engine.Call{RunID: "run-1", Seq: 1, Tool: "dpg.whoami", OnBehalfOf: "usr_ana"}); err != nil || res.Failed {
			t.Fatalf("call = %+v, %v", res, err)
		}
	}
	call()
	creds["usr_ana"] = domain.MCPCredentials{Token: "second"}
	call()
	want := []string{"Bearer first → Bearer first", "Bearer second → Bearer second"}
	if got := ids.callsBy(); !slices.Equal(got, want) {
		t.Errorf("calls = %v, want the second on a session the new token opened", got)
	}
}

// rotatingStore keeps a person's refreshed grant as the store does: under the
// generation the person's write gave it (domain.KeptGeneration).
type rotatingStore struct {
	mu    sync.Mutex
	creds personalCreds
}

func (s *rotatingStore) MCPPersonalCredential(
	ctx context.Context, at domain.Scope, name string, who domain.UserID,
) (domain.MCPCredentials, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds.MCPPersonalCredential(ctx, at, name, who)
}

func (s *rotatingStore) RefreshMCPServerOAuth(_ context.Context, r admin.MCPOAuthRefresh) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := domain.KeptGeneration(r.Was, r.Next)
	s.creds[r.Principal] = domain.MCPCredentials{OAuth: &kept}
	return nil
}

// The worker renewing a person's access token is nobody changing anything:
// the session that grant opened carries on.
func TestConnectServer_aPersonsGrantRenewedByTheWorker_keepsTheirSession(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	var issued int
	var mu sync.Mutex
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		issued++
		n := issued
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("ana-%d", n), "refresh_token": "refresh", "expires_in": 1,
		})
	}))
	t.Cleanup(idp.Close)
	store := &rotatingStore{creds: personalCreds{"usr_ana": {OAuth: &domain.MCPOAuthGrant{
		AccessToken: "ana-0", RefreshToken: "refresh", TokenURL: idp.URL,
		ExpiresAtUnix: time.Now().Add(-time.Minute).Unix(), Generation: "gen-1",
	}}}}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	server := domain.MCPServer{Name: "dpg", Transport: domain.TransportHTTP, URL: url, Enabled: true}
	if err := connectServer(t.Context(), catalog, server, domain.MCPCredentials{Token: "shared"}, store,
		store, credentialPolicy{requirePersonal: true}, nil); err != nil {
		t.Fatalf("connectServer: %v", err)
	}
	for range 2 {
		if res, err := catalog.Invoke(t.Context(), engine.Call{RunID: "run-1", Seq: 1, Tool: "dpg.whoami", OnBehalfOf: "usr_ana"}); err != nil || res.Failed {
			t.Fatalf("call = %+v, %v", res, err)
		}
	}
	if got := ids.initializedWith(); len(got) != 2 {
		t.Errorf("sessions initialized with %v, want the shared discovery session and one of Ana's", got)
	}
}

// unreadableCreds is a personal store that cannot be read.
type unreadableCreds struct{}

func (unreadableCreds) MCPPersonalCredential(
	context.Context, domain.Scope, string, domain.UserID,
) (domain.MCPCredentials, bool, error) {
	return domain.MCPCredentials{}, false, errors.New("vault: sealed")
}

// A credential that cannot be read is said as such, with the code the trail
// and the metrics read, whichever path reads it first.
func TestConnectServer_aPersonalCredentialThatCannotBeRead_keepsItsCode(t *testing.T) {
	t.Parallel()
	_, url := newIdentityServer(t)
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	server := domain.MCPServer{Name: "dpg", Transport: domain.TransportHTTP, URL: url, Enabled: true}
	if err := connectServer(t.Context(), catalog, server, domain.MCPCredentials{Token: "shared"}, nil,
		unreadableCreds{}, credentialPolicy{requirePersonal: true}, nil); err != nil {
		t.Fatalf("connectServer: %v", err)
	}
	res, err := catalog.Invoke(t.Context(), engine.Call{RunID: "run-1", Seq: 1, Tool: "dpg.whoami", OnBehalfOf: "usr_ana"})
	if err == nil || res.ErrorCode != tools.CodeMCPPersonalCredentialRead {
		t.Fatalf("call = %q, %v; want %s", res.ErrorCode, err, tools.CodeMCPPersonalCredentialRead)
	}
}
