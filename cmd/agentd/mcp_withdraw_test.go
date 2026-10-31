package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

func connectedCRM(t *testing.T, configured []domain.MCPServer) (*reconciler, *recordingPublisher) {
	t.Helper()
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "crm", Transport: domain.TransportHTTP, URL: "https://tools.example.com/mcp",
		Enabled: true, UpdatedAt: time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC),
	}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	if err := catalog.AddServer(ctx, "crm", &testSession{tools: []*mcp.Tool{{Name: "lookup"}}}, nil); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	publisher := &recordingPublisher{}
	r := newReconciler(catalog, &probeServers{servers: configured}, recordingHealth{}).publishingTo(publisher)
	r.connected["crm"] = fingerprint(server)
	r.connectTo = func(
		context.Context, *tools.Catalog, domain.MCPServer,
		domain.MCPCredentials, OAuthGrantStore, MCPUserCredentialStore, credentialPolicy,
		stdioEgressObserver,
	) error {
		return errors.New("dial timeout")
	}
	return r, publisher
}

// Removed or switched off, a server leaves this worker with nothing new to
// connect — and that is still a change worth publishing, because its tools
// are on the console until somebody says it is gone.
func TestReconciler_aServerTheConfigurationNoLongerWants_isWithdrawnInAPublication(t *testing.T) {
	t.Parallel()
	r, publisher := connectedCRM(t, nil)

	r.reconcile(context.Background())

	if publisher.calls != 1 || !slices.Equal(publisher.last.Withdrawn, []string{"crm"}) {
		t.Fatalf("published %d times, last %+v; want one publication withdrawing crm", publisher.calls, publisher.last)
	}
	if slices.Contains(publisher.last.Servers, "crm") {
		t.Fatalf("Servers = %v, want crm no longer spoken for", publisher.last.Servers)
	}
}

// A server still wanted but not reachable right now — its address changed
// and the reconnect failed — is not withdrawn: nothing says it stopped
// offering its tools, only that this worker cannot see them.
func TestReconciler_aWantedServerThatFailsToReconnect_isNotWithdrawn(t *testing.T) {
	t.Parallel()
	moved := domain.MCPServer{
		Name: "crm", Transport: domain.TransportHTTP, URL: "https://tools.example.com/v2/mcp",
		Enabled: true, UpdatedAt: time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC),
	}
	r, publisher := connectedCRM(t, []domain.MCPServer{moved})

	r.reconcile(context.Background())

	if len(publisher.last.Withdrawn) != 0 || slices.Contains(publisher.last.Servers, "crm") {
		t.Fatalf("published %+v; want crm neither withdrawn nor spoken for", publisher.last)
	}
}
