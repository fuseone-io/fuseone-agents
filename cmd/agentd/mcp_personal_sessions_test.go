package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
		handler.ServeHTTP(w, r)
		ids.observe(w.Header().Get("Mcp-Session-Id"), r.Header.Get("Mcp-Session-Id"), auth, body)
	}))
	t.Cleanup(remote.Close)
	return ids, remote.URL
}

func (s *identityServer) observe(answered, asked, auth string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case bytes.Contains(body, []byte(`"method":"initialize"`)):
		s.openedBy[answered] = auth
	case bytes.Contains(body, []byte(`"method":"tools/call"`)):
		s.calls[asked] = append(s.calls[asked], auth)
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
