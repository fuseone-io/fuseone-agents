package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

// closedSession is a session whose connection the SDK has given up on.
type closedSession struct{ testSession }

func (s *closedSession) CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return nil, fmt.Errorf(`%w: calling "tools/call": client is closing`, mcp.ErrConnectionClosed)
}

func deadSessionFixture(t *testing.T) (*reconciler, *tools.Catalog, *int, recordingHealth) {
	t.Helper()
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "grafana", Transport: domain.TransportHTTP, URL: "https://grafana.example.com/mcp",
		Enabled: true, UpdatedAt: time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC),
	}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	if err := catalog.AddServer(ctx, "grafana",
		&closedSession{testSession{tools: []*mcp.Tool{{Name: "query_loki_logs"}}}}, nil); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	health := recordingHealth{}
	r := newReconciler(catalog, &probeServers{servers: []domain.MCPServer{server}}, health)
	r.connected["grafana"] = fingerprint(server)

	connects := 0
	r.connectTo = func(
		ctx context.Context, catalog *tools.Catalog, server domain.MCPServer,
		_ domain.MCPCredentials, _ OAuthGrantStore, _ MCPUserCredentialStore, _ credentialPolicy,
		_ stdioEgressObserver,
	) error {
		connects++
		return catalog.AddServer(ctx, server.Name,
			&testSession{tools: []*mcp.Tool{{Name: "query_loki_logs"}}}, server.Surface)
	}
	return r, catalog, &connects, health
}

func TestReconciler_aServerWhoseSessionDied_isReconnected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r, catalog, connects, health := deadSessionFixture(t)

	_, _ = catalog.Invoke(ctx, engine.Call{RunID: "run-1", Seq: 1, Tool: "grafana.query_loki_logs"})
	r.reconcile(ctx)

	if *connects != 1 {
		t.Fatalf("connects = %d, want the dead server reconnected once", *connects)
	}
	if !catalog.Connected("grafana") {
		t.Error("grafana is still disconnected after the reconcile")
	}
	if seen := health["grafana"]; !seen.Reachable {
		t.Errorf("health = %+v, want the reconnect observed as reachable", seen)
	}
}

func TestReconciler_aLiveSession_isLeftAlone(t *testing.T) {
	t.Parallel()
	r, _, connects, _ := deadSessionFixture(t)

	r.reconcile(context.Background())

	if *connects != 0 {
		t.Fatalf("connects = %d, want a live session left alone", *connects)
	}
}
