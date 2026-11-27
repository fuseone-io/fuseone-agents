package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

/*
A server that takes each person's own credential, and that nothing else can
discover, is discovered as whoever connected their account to it last (#483,
delivery 3). Before, its catalogue was learned with the shared credential or
not at all, and a server that refuses everybody but a person stayed with no
tools, so no person could ever call one.

The order: a shared grant that cannot be used goes straight to the person; no
shared credential is tried as nobody first, since a server may discover
anonymously; one that answers keeps the session it opened, and only a refusal
(401, 403) goes to the person. A server that is down is down.
*/

// discoveryStore is the administration as the reconciler reads it: the
// server, its shared credential, each person's, and who discovers.
type discoveryStore struct {
	mu            sync.Mutex
	server        domain.MCPServer
	shared        domain.MCPCredentials
	creds         personalCreds
	discoverer    domain.UserID
	discovererErr error
	lookups       int
	trailErr      error
	recorded      []admin.MCPDiscoveredOnBehalf
}

func (s *discoveryStore) MCPServers(context.Context) ([]domain.MCPServer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return []domain.MCPServer{s.server}, nil
}

func (s *discoveryStore) MCPServerCredentials(context.Context, domain.MCPServer) (domain.MCPCredentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shared, nil
}

func (s *discoveryStore) MCPPersonalCredential(
	ctx context.Context, at domain.Scope, name string, who domain.UserID,
) (domain.MCPCredentials, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds.MCPPersonalCredential(ctx, at, name, who)
}

func (s *discoveryStore) MCPPersonalDiscoverer(context.Context, domain.Scope, string) (domain.UserID, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lookups++
	return s.discoverer, s.discoverer != "", s.discovererErr
}

func (s *discoveryStore) RecordMCPDiscoveredOnBehalf(_ context.Context, d admin.MCPDiscoveredOnBehalf) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trailErr != nil {
		return s.trailErr
	}
	s.recorded = append(s.recorded, d)
	return nil
}

func (s *discoveryStore) update(f func(*discoveryStore)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

func (s *discoveryStore) lookupCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lookups
}

func (s *discoveryStore) trail() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.recorded))
	for _, d := range s.recorded {
		out = append(out, d.Server+" as "+string(d.Principal)+" by "+d.Holder)
	}
	return out
}

type everyonePersonal struct{}

func (everyonePersonal) RequiresPersonalCredential(string) bool { return true }

type discoveryWorld struct {
	store   *discoveryStore
	catalog *tools.Catalog
	health  recordingHealth
	r       *reconciler
}

// withDiscovery is a worker reconciling dpg at url, a server that takes each
// person's credential, with no shared credential; Ana and Bia have their own,
// and Ana discovers.
func withDiscovery(t *testing.T, url string) discoveryWorld {
	t.Helper()
	store := &discoveryStore{
		server: domain.MCPServer{Name: "dpg", Transport: domain.TransportHTTP, URL: url, Enabled: true},
		creds: personalCreds{
			"usr_ana": {Token: "ana"},
			"usr_bia": {Token: "bia"},
		},
		discoverer: "usr_ana",
	}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	health := recordingHealth{}
	r := newReconciler(catalog, store, health).withCredentialPolicy(everyonePersonal{})
	r.holder = "worker-1"
	return discoveryWorld{store: store, catalog: catalog, health: health, r: r}
}

func (w discoveryWorld) invoke(t *testing.T, who domain.UserID) (engine.ToolResult, error) {
	t.Helper()
	return w.catalog.Invoke(t.Context(), engine.Call{RunID: "run-1", Seq: 1, Tool: "dpg.whoami", OnBehalfOf: who})
}

func TestReconcile_aServerOnlyAPersonCanDiscover_isDiscoveredAsWhoeverConnectedLast(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	w := withDiscovery(t, url)

	w.r.reconcile(t.Context())

	if !w.catalog.Connected("dpg") || w.catalog.CountFrom("dpg") != 1 {
		t.Fatalf("dpg connected=%v with %d tools, want its catalogue learned", w.catalog.Connected("dpg"), w.catalog.CountFrom("dpg"))
	}
	if got := ids.initializedWith(); !slices.Equal(got, []string{"Bearer ana"}) {
		t.Errorf("sessions initialized with %v, want discovery as Ana", got)
	}
	if h := w.health["dpg"]; !h.Reachable || h.ToolCount != 1 {
		t.Errorf("health = %+v, want reachable with its tool", h)
	}
}

// The session learned as Ana answers nobody's call: a run with no person, a
// schedule's, would otherwise run as her.
func TestReconcile_discoveredAsAPerson_aRunWithNoPersonNeverRunsOnThatSession(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	w := withDiscovery(t, url)
	w.r.reconcile(t.Context())

	res, err := w.invoke(t, "")
	if err == nil || !res.Failed {
		t.Fatalf("call with no person = %+v, %v; want it refused", res, err)
	}
	assertFailureCode(t, err, tools.CodeMCPPersonalCredentialCaller)
	if got := ids.callsBy(); len(got) != 0 {
		t.Errorf("calls reached the server: %v", got)
	}

	if res, err := w.invoke(t, "usr_bia"); err != nil || res.Failed {
		t.Fatalf("call as Bia = %+v, %v", res, err)
	}
	if got := ids.callsBy(); !slices.Equal(got, []string{"Bearer bia → Bearer bia"}) {
		t.Errorf("calls = %v, want Bia's on a session her own token opened", got)
	}
}

func TestDiscoveryOnly_refusesACallBeforeSendingIt(t *testing.T) {
	t.Parallel()
	session := &listOnlySession{}
	_, err := discoveryOnly{Session: session, server: "dpg"}.CallTool(t.Context(), &mcp.CallToolParams{Name: "whoami"})
	assertFailureCode(t, err, tools.CodeMCPPersonalCredentialCaller)
	if session.calls != 0 {
		t.Errorf("the call reached the session %d times", session.calls)
	}
}

type listOnlySession struct{ calls int }

func (c *listOnlySession) ListTools(context.Context, *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	return &mcp.ListToolsResult{}, nil
}

func (c *listOnlySession) CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	c.calls++
	return &mcp.CallToolResult{}, nil
}

func (c *listOnlySession) Close() error { return nil }

// A server that discovers anonymously keeps doing so: no shared credential
// does not mean discovery needs one, nor a person's.
func TestReconcile_aServerThatDiscoversAnonymously_isDiscoveredAsNobody(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var seen []string
	server := mcp.NewServer(&mcp.Implementation{Name: "open", Version: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "whoami"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{}, nil, nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	remote := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		handler.ServeHTTP(rw, r)
	}))
	t.Cleanup(remote.Close)
	w := withDiscovery(t, remote.URL)

	w.r.reconcile(t.Context())

	if !w.catalog.Connected("dpg") {
		t.Fatal("the anonymous server did not connect")
	}
	mu.Lock()
	defer mu.Unlock()
	for _, auth := range seen {
		if auth != "" {
			t.Errorf("discovery sent %q, want nothing", auth)
		}
	}
	if n := w.store.lookupCount(); n != 0 {
		t.Errorf("asked who discovers %d times, want never", n)
	}
}

func TestReconcile_aSharedCredentialThatWorks_discoversWithIt(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	w := withDiscovery(t, url)
	w.store.update(func(s *discoveryStore) { s.shared = domain.MCPCredentials{Token: "shared"} })

	w.r.reconcile(t.Context())

	if got := ids.initializedWith(); !slices.Equal(got, []string{"Bearer shared"}) {
		t.Errorf("sessions initialized with %v, want the shared credential's alone", got)
	}
	if n := w.store.lookupCount(); n != 0 {
		t.Errorf("asked who discovers %d times, want never", n)
	}
}

// dpg's own case: a grant that holds no token nor a way to get one is not
// tried at all, as nobody or as itself.
func TestReconcile_aSharedGrantThatCannotBeUsed_goesStraightToThePerson(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	w := withDiscovery(t, url)
	w.store.update(func(s *discoveryStore) {
		s.shared = domain.MCPCredentials{OAuth: &domain.MCPOAuthGrant{TokenURL: "https://issuer.example/token"}}
	})

	w.r.reconcile(t.Context())

	if got := ids.initializedWith(); !slices.Equal(got, []string{"Bearer ana"}) {
		t.Errorf("sessions initialized with %v, want discovery as Ana", got)
	}
	ids.mu.Lock()
	defer ids.mu.Unlock()
	if ids.anonymous != 0 {
		t.Errorf("%d requests went out as nobody first", ids.anonymous)
	}
}

func TestReconcile_aServerThatIsDown_isNotDiscoveredAsAPerson(t *testing.T) {
	t.Parallel()
	failing := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		http.Error(rw, "down", http.StatusBadGateway)
	}))
	t.Cleanup(failing.Close)
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()

	for name, url := range map[string]string{"5xx": failing.URL, "unreachable": closed.URL} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := withDiscovery(t, url)
			w.r.reconcile(t.Context())
			if w.catalog.Connected("dpg") || w.health["dpg"].Reachable {
				t.Fatal("a server that is down connected")
			}
			if n := w.store.lookupCount(); n != 0 {
				t.Errorf("asked who discovers %d times; a server that is down needs no person", n)
			}
		})
	}
}

func TestReconcile_nobodyConnected_saysTheServerWaitsForAPerson(t *testing.T) {
	t.Parallel()
	_, url := newIdentityServer(t)
	w := withDiscovery(t, url)
	w.store.update(func(s *discoveryStore) { s.discoverer = "" })

	w.r.reconcile(t.Context())

	h := w.health["dpg"]
	if w.catalog.Connected("dpg") || h.Reachable {
		t.Fatalf("connected with nobody to discover as: %+v", h)
	}
	if !strings.Contains(h.Detail, "no person has connected") {
		t.Errorf("detail = %q, want it to say no person has connected", h.Detail)
	}
}

// Not knowing who could discover is a failure to read, never nobody.
func TestReconcile_theStoreFailingToSayWhoDiscovers_isNotNobody(t *testing.T) {
	t.Parallel()
	_, url := newIdentityServer(t)
	w := withDiscovery(t, url)
	w.store.update(func(s *discoveryStore) { s.discoverer, s.discovererErr = "", errors.New("settings unreachable") })

	w.r.reconcile(t.Context())

	h := w.health["dpg"]
	if h.Reachable || strings.Contains(h.Detail, "no person has connected") || !strings.Contains(h.Detail, "settings unreachable") {
		t.Errorf("health = %+v, want the read's failure, not nobody", h)
	}

	// A failed read is tried again on the next pass.
	w.store.update(func(s *discoveryStore) { s.discoverer, s.discovererErr = "usr_ana", nil })
	w.r.reconcile(t.Context())
	if !w.catalog.Connected("dpg") || !w.health["dpg"].Reachable {
		t.Errorf("dpg not discovered on the pass after the read recovered: %+v", w.health["dpg"])
	}
}

func TestReconcile_theDiscovererChangingTheirCredential_rediscoversWithTheNewOne(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	w := withDiscovery(t, url)
	w.r.reconcile(t.Context())

	w.store.update(func(s *discoveryStore) { s.creds["usr_ana"] = domain.MCPCredentials{Token: "ana-2"} })
	w.r.reconcile(t.Context())

	if got := ids.initializedWith(); !slices.Equal(got, []string{"Bearer ana", "Bearer ana-2"}) {
		t.Errorf("sessions initialized with %v, want discovery again with Ana's new token", got)
	}
	if !w.catalog.Connected("dpg") {
		t.Error("dpg is not connected after the rediscovery")
	}
}

func TestReconcile_anotherPersonDiscoveringNow_handsDiscoveryOver(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	w := withDiscovery(t, url)
	w.r.reconcile(t.Context())

	w.store.update(func(s *discoveryStore) {
		delete(s.creds, "usr_ana")
		s.discoverer = "usr_bia"
	})
	w.r.reconcile(t.Context())

	if got := ids.initializedWith(); !slices.Equal(got, []string{"Bearer ana", "Bearer bia"}) {
		t.Errorf("sessions initialized with %v, want discovery handed to Bia", got)
	}
}

func TestReconcile_theLastPersonLeaving_disconnectsTheServer(t *testing.T) {
	t.Parallel()
	_, url := newIdentityServer(t)
	w := withDiscovery(t, url)
	w.r.reconcile(t.Context())

	w.store.update(func(s *discoveryStore) { s.creds, s.discoverer = personalCreds{}, "" })
	w.r.reconcile(t.Context())

	if w.catalog.Connected("dpg") || w.health["dpg"].Reachable {
		t.Error("dpg stayed connected as a person who left")
	}
}

// Once for the person, not for each session: Ana's new token is discovered
// again, and she is who this worker discovered as all along.
func TestReconcile_discoveredAsAPerson_recordsItOnceByIds(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	w := withDiscovery(t, url)

	w.r.reconcile(t.Context())
	w.store.update(func(s *discoveryStore) { s.creds["usr_ana"] = domain.MCPCredentials{Token: "ana-2"} })
	w.r.reconcile(t.Context())
	w.r.reconcile(t.Context())

	if got := ids.initializedWith(); len(got) != 2 {
		t.Fatalf("sessions initialized with %v, want Ana's discovery twice", got)
	}

	if got := w.store.trail(); !slices.Equal(got, []string{"dpg as usr_ana by worker-1"}) {
		t.Errorf("trail = %v, want Ana's discovery recorded once", got)
	}
}

// A record that could not be written is owed, and a person who discovers
// after does not take the place of one still owed.
func TestReconcile_aTrailThatFails_isWrittenOnALaterPass_everyRecordOfIt(t *testing.T) {
	t.Parallel()
	_, url := newIdentityServer(t)
	w := withDiscovery(t, url)
	w.store.update(func(s *discoveryStore) { s.trailErr = errors.New("trail unreachable") })
	w.r.reconcile(t.Context())

	w.store.update(func(s *discoveryStore) {
		delete(s.creds, "usr_ana")
		s.discoverer = "usr_bia"
	})
	w.r.reconcile(t.Context())
	if got := w.store.trail(); len(got) != 0 {
		t.Fatalf("trail = %v while it fails", got)
	}

	w.store.update(func(s *discoveryStore) { s.trailErr = nil })
	w.r.reconcile(t.Context())

	got := w.store.trail()
	slices.Sort(got)
	if want := []string{"dpg as usr_ana by worker-1", "dpg as usr_bia by worker-1"}; !slices.Equal(got, want) {
		t.Errorf("trail = %v, want %v", got, want)
	}
}

// Not knowing who discovers now is not a change of who does.
func TestReconcile_theStoreFailingAfterDiscovery_keepsTheSession(t *testing.T) {
	t.Parallel()
	ids, url := newIdentityServer(t)
	w := withDiscovery(t, url)
	w.r.reconcile(t.Context())

	w.store.update(func(s *discoveryStore) { s.discovererErr = errors.New("settings unreachable") })
	w.r.reconcile(t.Context())

	if !w.catalog.Connected("dpg") {
		t.Error("the session ended on a failed read")
	}
	if got := ids.initializedWith(); !slices.Equal(got, []string{"Bearer ana"}) {
		t.Errorf("sessions initialized with %v, want Ana's first one kept", got)
	}
}

// A person's request refused is the person's refusal, said as the server said
// it: only a request made for nobody is a discovery that wants a person.
func TestRefusingDiscovery_aPersonsRequestRefused_passesAsTheServerAnswered(t *testing.T) {
	t.Parallel()
	remote := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		http.Error(rw, "no", http.StatusUnauthorized)
	}))
	t.Cleanup(remote.Close)
	rt := refusingDiscovery{base: http.DefaultTransport}

	asPerson, _ := http.NewRequestWithContext(tools.WithInvocation(tools.WithCaller(t.Context(), "usr_ana")), http.MethodPost, remote.URL, nil)
	res, err := rt.RoundTrip(asPerson)
	if err != nil || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("person's request = %v, %v; want the 401 itself", res, err)
	}
	_ = res.Body.Close()

	asNobody, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, remote.URL, nil)
	if _, err := rt.RoundTrip(asNobody); !errors.Is(err, errDiscoveryRefused) {
		t.Errorf("nobody's request err = %v, want errDiscoveryRefused", err)
	}
}
