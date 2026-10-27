package tools_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

// closedErr is what the SDK returns for every call on a session whose
// connection has failed for good.
var closedErr = fmt.Errorf(`%w: calling "tools/call": client is closing: sending "notifications/cancelled": Bad Request`,
	mcp.ErrConnectionClosed)

func TestInvoke_connectionClosed_serverStopsCountingAsConnected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	srv := lookupServer()
	srv.err = closedErr
	c, _ := catalogWith(t, srv)

	if _, err := c.Invoke(ctx, engine.Call{RunID: "run-1", Seq: 1, Tool: "crm.lookup"}); err == nil {
		t.Fatal("Invoke succeeded on a closed connection")
	}
	if c.Connected("crm") {
		t.Error("Connected(crm) = true after the session died, want false so it is reconnected")
	}
	if !srv.closed {
		t.Error("dead session was not closed")
	}
	if _, ok := c.Lookup("crm.lookup"); !ok {
		t.Error("tools of a dead session were dropped; they should stay until the reconnect replaces them")
	}
}

func TestInvoke_otherTransportFailure_keepsTheSession(t *testing.T) {
	t.Parallel()

	srv := lookupServer()
	srv.err = errors.New("Bad Request: Unsupported protocol version")
	c, _ := catalogWith(t, srv)

	_, _ = c.Invoke(context.Background(), engine.Call{RunID: "run-1", Seq: 1, Tool: "crm.lookup"})

	if !c.Connected("crm") {
		t.Error("Connected(crm) = false after a failure that leaves the connection usable")
	}
}

// reconnectingServer replaces itself in the catalogue while its own call is in
// flight, then reports its connection closed — the race between a slow call on
// a dying session and the reconciler installing a fresh one.
type reconnectingServer struct {
	*fakeServer
	catalog     *tools.Catalog
	replacement *fakeServer
}

func (r *reconnectingServer) CallTool(ctx context.Context, _ *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	if err := r.catalog.AddServer(ctx, "crm", r.replacement, nil); err != nil {
		return nil, err
	}
	return nil, closedErr
}

func TestInvoke_connectionClosedAfterReconnect_keepsTheNewSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	c := tools.NewCatalog(engine.NewMemoryContent())
	stale := &reconnectingServer{fakeServer: lookupServer(), catalog: c, replacement: lookupServer()}
	if err := c.AddServer(ctx, "crm", stale, nil); err != nil {
		t.Fatalf("AddServer: %v", err)
	}

	_, _ = c.Invoke(ctx, engine.Call{RunID: "run-1", Seq: 1, Tool: "crm.lookup"})

	if !c.Connected("crm") {
		t.Fatal("a stale session's failure disconnected the session that replaced it")
	}
	if _, err := c.Invoke(ctx, engine.Call{RunID: "run-1", Seq: 2, Tool: "crm.lookup"}); err != nil {
		t.Fatalf("Invoke on the replacement: %v", err)
	}
	if stale.replacement.closed {
		t.Error("the replacement session was closed")
	}
}

func TestRemoveServer_afterItsSessionDied_dropsItsTools(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	srv := lookupServer()
	srv.err = closedErr
	c, _ := catalogWith(t, srv)
	_, _ = c.Invoke(ctx, engine.Call{RunID: "run-1", Seq: 1, Tool: "crm.lookup"})

	if err := c.RemoveServer("crm"); err != nil {
		t.Fatalf("RemoveServer: %v", err)
	}
	if _, ok := c.Lookup("crm.lookup"); ok {
		t.Error("a removed server's tools survived because its session had already died")
	}
}

// Session answers by registration, never by prefix: it is what lets the
// engine tell a dead session's tool (entry kept, session gone) from a
// native connector's tool that merely shares a namespace with a server
// (#150 round 3).
func TestSession_ownershipComesFromRegistrationNotFromThePrefix(t *testing.T) {
	t.Parallel()

	srv := lookupServer()
	c, _ := catalogWith(t, srv)

	server, live, owned := c.Session("crm.lookup")
	if server != "crm" || !live || !owned {
		t.Errorf("Session(crm.lookup) = %q/%v/%v, want crm, live, owned", server, live, owned)
	}
	if _, _, owned := c.Session("crm.something-never-registered"); owned {
		t.Error("an unregistered tool reads as owned because its prefix matches a server")
	}

	srv.err = closedErr
	ctx := context.Background()
	if _, err := c.Invoke(ctx, engine.Call{RunID: "run-1", Seq: 1, Tool: "crm.lookup"}); err == nil {
		t.Fatal("Invoke succeeded on a closed connection")
	}
	server, live, owned = c.Session("crm.lookup")
	if server != "crm" || live || !owned {
		t.Errorf("Session after death = %q/%v/%v, want crm, not live, still owned", server, live, owned)
	}
}
