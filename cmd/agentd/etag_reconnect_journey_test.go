package main

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/tools"
)

/*
What a worker does about a write, through the route and the reconciler serve runs.

updated_at is what a worker compares, so a save with no change used to restart the session
on every worker and the socket with it. A `terraform apply` that changes nothing must not.
A real change still has to reach them, or a rotated credential would never be picked up.
*/
func TestReconciler_anIdenticalPut_keepsTheSession_andARealChangeReplacesIt(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Isolated(t)
	w := readBackWorldOn(t, pool)
	path := "/admin/integrations/mcp-servers/etag-reconnect"
	body := func(url, token string) map[string]any {
		return map[string]any{"transport": "http", "url": url, "token": token}
	}
	put := func(b map[string]any) {
		t.Helper()
		if got := w.call(t, etagCall{method: http.MethodPut, path: path, body: b}); got.Code != http.StatusNoContent {
			t.Fatalf("PUT = %d: %s", got.Code, got.Body)
		}
	}
	const urlA, urlB = "https://mcp.example.com/a", "https://mcp.example.com/b"
	put(body(urlA, "ETAG-TOKEN-ONE"))

	r := newReconciler(tools.NewCatalog(engine.NewMemoryContent()), admin.NewIntegrations(pool, w.store), recordingHealth{})
	connects := 0
	r.connectTo = func(
		ctx context.Context, catalog *tools.Catalog, server domain.MCPServer,
		_ domain.MCPCredentials, _ OAuthGrantStore, _ MCPUserCredentialStore, _ credentialPolicy,
		_ stdioEgressObserver,
	) error {
		connects++
		return catalog.AddServer(ctx, server.Name, &testSession{tools: []*mcp.Tool{{Name: "lookup"}}}, server.Surface)
	}
	r.reconcile(ctx)
	if connects != 1 {
		t.Fatalf("connects = %d after the first pass", connects)
	}

	for _, step := range []struct {
		name string
		body map[string]any
		want int
	}{
		{"the same again", body(urlA, "ETAG-TOKEN-ONE"), 1},
		{"another address", body(urlB, "ETAG-TOKEN-ONE"), 2},
		{"the same again", body(urlB, "ETAG-TOKEN-ONE"), 2},
		{"another credential", body(urlB, "ETAG-TOKEN-TWO"), 3},
	} {
		put(step.body)
		r.reconcile(ctx)
		if connects != step.want {
			t.Errorf("%s: connects = %d, want %d", step.name, connects, step.want)
		}
	}
}

func TestSlackSocket_anIdenticalPut_keepsTheSocket_andARealChangeReplacesIt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := testdb.Isolated(t)
	w := readBackWorldOn(t, pool)
	path := "/admin/channels/etag-socket"
	body := func(workspace, appToken string) map[string]any {
		return map[string]any{
			"kind": "slack", "workspace": workspace, "deliveryMode": "socket",
			"token": "xoxb-etag", "appToken": appToken,
		}
	}
	put := func(b map[string]any) {
		t.Helper()
		if got := w.call(t, etagCall{method: http.MethodPut, path: path, body: b}); got.Code != http.StatusNoContent {
			t.Fatalf("PUT = %d: %s", got.Code, got.Body)
		}
	}
	put(body("acme", "xapp-one"))

	var dials atomic.Int32
	m := testSocketManager(newFakeSocket)
	build := m.dial
	m.dial = func(ctx context.Context, url string) (slackSocketConn, error) {
		dials.Add(1)
		return build(ctx, url)
	}
	m.settings = w.store
	defer m.stopAll()
	settled := func(want int32) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for dials.Load() < want && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		// Long enough for a socket that should not have opened to have opened.
		time.Sleep(30 * time.Millisecond)
		if got := dials.Load(); got != want {
			t.Fatalf("dials = %d, want %d", got, want)
		}
	}
	m.sync(ctx)
	settled(1)

	for _, step := range []struct {
		name string
		body map[string]any
		want int32
	}{
		{"the same again", body("acme", "xapp-one"), 1},
		{"another workspace", body("acme two", "xapp-one"), 2},
		{"the same again", body("acme two", "xapp-one"), 2},
		{"another app token", body("acme two", "xapp-two"), 3},
	} {
		t.Log(step.name)
		put(step.body)
		m.sync(ctx)
		settled(step.want)
	}
	if _, err := w.store.Get(ctx, settings.ScopeInstallation, domain.Scope{}, "channel", "etag-socket"); err != nil {
		t.Fatalf("Get: %v", err)
	}
}
