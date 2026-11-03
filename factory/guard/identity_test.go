package guard

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tokenBox is the bearer a test client sends; a test can swap it mid-session,
// as the platform does when it mints a fresh installation token.
type tokenBox struct {
	mu    sync.Mutex
	token string
}

func (b *tokenBox) set(token string) { b.mu.Lock(); b.token = token; b.mu.Unlock() }
func (b *tokenBox) get() string      { b.mu.Lock(); defer b.mu.Unlock(); return b.token }

type bearerTransport struct {
	box    *tokenBox
	extra  http.Header
	parent http.RoundTripper
}

func (t bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.box.get())
	for k, v := range t.extra {
		r.Header[k] = v
	}
	return t.parent.RoundTrip(r)
}

type idHarness struct {
	*boardHarness
	srv   *httptest.Server
	clock *fakeClock
}

// newIDHarness serves every role in pass-through mode over real HTTP. The
// fake knows each role's bot token, a second token for Triage's bot (one
// re-minted), and a stranger: a valid token whose bot no role lists.
func newIDHarness(t *testing.T, lane Lane) *idHarness {
	t.Helper()
	bh := newBoardHarness(t, lane)
	bh.fake.bots["triage-token-2"] = roleBot[RoleTriage]
	bh.fake.bots["stranger-token"] = 777
	bots := map[int64]Role{}
	for role, id := range roleBot {
		bots[id] = role
	}
	cfg := Config{Auth: AuthPassThrough, RoleBots: bots, Board: bh.cfg}
	gh := NewClient(ClientOptions{GraphQLURL: bh.srv.URL + "/graphql", HTTP: bh.srv.Client(), Tokens: RequestTokenSource{}})
	clock := &fakeClock{now: time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)}
	handler, err := NewPassThroughHandler(cfg, gh, clock)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &idHarness{boardHarness: bh, srv: srv, clock: clock}
}

func (h *idHarness) connectAt(path string, box *tokenBox, extra http.Header) (*mcp.ClientSession, error) {
	transport := &mcp.StreamableClientTransport{
		Endpoint:             h.srv.URL + path,
		HTTPClient:           &http.Client{Transport: bearerTransport{box: box, extra: extra, parent: http.DefaultTransport}},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "fuseone", Version: "test"}, nil)
	cs, err := client.Connect(h.t.Context(), transport, nil)
	if err == nil {
		h.t.Cleanup(func() { _ = cs.Close() })
	}
	return cs, err
}

func (h *idHarness) connectAs(box *tokenBox) *mcp.ClientSession {
	h.t.Helper()
	cs, err := h.connectAt("/", box, nil)
	if err != nil {
		h.t.Fatalf("connect as %s: %v", box.get(), err)
	}
	return cs
}

func (h *idHarness) status(header string) int {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/", strings.NewReader(`{}`))
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("post: %v", err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestPassThrough_eachIdentity_listsExactlyItsSurface(t *testing.T) {
	t.Parallel()

	for role, want := range roleSurface {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			h := newIDHarness(t, LaneTriagem)
			got := listedNames(t, h.connectAs(&tokenBox{token: roleToken(role)}))
			if want = slices.Sorted(slices.Values(want)); !slices.Equal(got, want) {
				t.Fatalf("%s lists %v, want exactly %v", role, got, want)
			}
		})
	}
}

func TestPassThrough_withoutAnAcceptedIdentity_refusedBeforeAnyTool(t *testing.T) {
	t.Parallel()

	h := newIDHarness(t, LaneTriagem)
	for header, want := range map[string]int{
		"":                       http.StatusUnauthorized,
		"Bearer ":                http.StatusUnauthorized,
		"Basic dHJpYWdlLXRva2Vu": http.StatusUnauthorized,
		"Bearer forged-token":    http.StatusUnauthorized, // GitHub rejects it
		"Bearer stranger-token":  http.StatusForbidden,    // a real bot no role lists
	} {
		if got := h.status(header); got != want {
			t.Errorf("Authorization %q: status %d, want %d", header, got, want)
		}
	}
	if _, err := h.connectAt("/", &tokenBox{token: "stranger-token"}, nil); err == nil {
		t.Fatal("an unlisted identity opened a session")
	}
}

// The surface comes from the token's identity. A path or a header naming a
// wider role changes nothing.
func TestPassThrough_pathAndHeader_neverWidenTheSurface(t *testing.T) {
	t.Parallel()

	h := newIDHarness(t, LaneTriagem)
	extra := http.Header{"X-Factory-Role": {"merger"}, "X-Mcp-Tools": {"merge_if_ready"}}
	cs, err := h.connectAt("/merger", &tokenBox{token: roleToken(RoleDeveloper)}, extra)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if got := listedNames(t, cs); !slices.Equal(got, []string{ToolMoveCard}) {
		t.Fatalf("the Developer lists %v at /merger with a merger header, want only move_card", got)
	}
}

// The token used against GitHub is the bearer of the call itself: after the
// platform re-mints, the next call acts with the new token.
func TestPassThrough_call_actsWithItsOwnBearer(t *testing.T) {
	t.Parallel()

	h := newIDHarness(t, LaneTriagem)
	box := &tokenBox{token: roleToken(RoleTriage)}
	cs := h.connectAs(box)
	box.set("triage-token-2")
	res := mustCall(t, cs, ToolMoveCard, map[string]any{"item_id": testItem, "lane": "Preparado"})
	got := h.mutations()
	if res.IsError || len(got) != 1 || got[0].Token != "triage-token-2" {
		t.Fatalf("isError %v, mutations %+v, want one with the call's own token", res.IsError, got)
	}
}

// A session belongs to the identity that opened it. Another identity's
// bearer cannot ride it into tools its own surface lacks.
func TestPassThrough_sessionOfAnotherIdentity_unknown(t *testing.T) {
	t.Parallel()

	h := newIDHarness(t, LaneTriagem)
	box := &tokenBox{token: roleToken(RoleTriage)}
	cs := h.connectAs(box)
	box.set(roleToken(RoleDeveloper))
	args := map[string]any{"item_id": testItem, "work_type": "bug", "size": "S"}
	if !refusedCall(t, cs, ToolSetCardFields, args) || len(h.mutations()) != 0 {
		t.Fatalf("the Developer's token set fields through Triage's session: %+v", h.mutations())
	}
}

func TestPassThrough_identity_cachedForLessThanATokensLife(t *testing.T) {
	t.Parallel()

	h := newIDHarness(t, LaneTriagem)
	cs := h.connectAs(&tokenBox{token: roleToken(RoleQA)})
	listedNames(t, cs)
	listedNames(t, cs)
	viewers := func() int { h.fake.mu.Lock(); defer h.fake.mu.Unlock(); return h.fake.viewerCalls }
	if n := viewers(); n != 1 {
		t.Fatalf("viewer asked %d times within the TTL, want once", n)
	}
	h.clock.Advance(identityTTL)
	listedNames(t, cs)
	if n := viewers(); n != 2 || identityTTL >= time.Hour {
		t.Fatalf("viewer asked %d times after the TTL (%v), want it asked again, under an hour", n, identityTTL)
	}
}

// Over the wire too, each role moves only along its own row.
func TestPassThrough_transitions_followTheIdentitysRow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		role     Role
		from, to Lane
		want     Reason
	}{
		{RoleDeveloper, LaneTriagem, LanePreparado, ReasonTransitionRefused},
		{RoleQA, LaneDoing, LaneDone, ReasonTransitionRefused},
		{RoleTriage, LanePreparado, LaneTravado, ReasonTransitionRefused},
		{RoleReviewer, LaneTravado, LaneDoing, ReasonTravadoIsHuman},
		{RoleMerger, LaneDoing, LaneDone, ReasonTransitionRefused},
		{RoleMerger, LaneDoing, LaneTravado, ReasonMoved},
	}
	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			t.Parallel()
			h := newIDHarness(t, tc.from)
			res := mustCall(t, h.connectAs(&tokenBox{token: roleToken(tc.role)}), ToolMoveCard,
				map[string]any{"item_id": testItem, "lane": string(tc.to)})
			if got := structured[MoveResult](t, res); got.Reason != tc.want {
				t.Fatalf("%s %s -> %s = %+v, want %s", tc.role, tc.from, tc.to, got, tc.want)
			}
		})
	}
}
