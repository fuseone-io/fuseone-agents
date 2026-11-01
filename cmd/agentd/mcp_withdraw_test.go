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

var crmGeneration = time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)

func crmServer() domain.MCPServer {
	return domain.MCPServer{
		Name: "crm", Transport: domain.TransportHTTP, URL: "https://tools.example.com/mcp",
		Enabled: true, UpdatedAt: crmGeneration,
	}
}

// reconcilerOver is a reconciler over the given configuration, whose connect
// answers with one tool unless told to fail.
func reconcilerOver(t *testing.T, configured []domain.MCPServer, fail bool) (*reconciler, *recordingPublisher) {
	t.Helper()
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = catalog.Close() })
	publisher := &recordingPublisher{}
	r := newReconciler(catalog, &probeServers{servers: configured}, recordingHealth{}).publishingTo(publisher)
	r.connectTo = func(
		ctx context.Context, catalog *tools.Catalog, server domain.MCPServer,
		_ domain.MCPCredentials, _ OAuthGrantStore, _ MCPUserCredentialStore, _ credentialPolicy,
		_ stdioEgressObserver,
	) error {
		if fail {
			return errors.New("dial timeout")
		}
		return catalog.AddServer(ctx, server.Name, &testSession{tools: []*mcp.Tool{{Name: "lookup"}}}, nil)
	}
	return r, publisher
}

func connectedCRM(t *testing.T, configured []domain.MCPServer) (*reconciler, *recordingPublisher) {
	t.Helper()
	r, publisher := reconcilerOver(t, configured, true)
	if err := r.catalog.AddServer(context.Background(), "crm",
		&testSession{tools: []*mcp.Tool{{Name: "lookup"}}}, nil); err != nil {
		t.Fatalf("seed catalog: %v", err)
	}
	r.connected["crm"] = fingerprint(crmServer())
	r.generations["crm"] = crmGeneration
	return r, publisher
}

// A discovery is published with the configuration it was made under, so the
// administration can tell it from a discovery of a configuration that has
// since changed; a command-line server, which no configuration holds, is
// published as unmanaged.
func TestReconciler_publication_carriesEachServersConfigurationOrUnmanaged(t *testing.T) {
	t.Parallel()
	r, publisher := reconcilerOver(t, []domain.MCPServer{crmServer()}, false)
	if err := r.catalog.AddServer(context.Background(), "local",
		&testSession{tools: []*mcp.Tool{{Name: "run"}}}, nil); err != nil {
		t.Fatalf("connect the command-line server: %v", err)
	}
	r.hold(domain.MCPServer{Name: "local"})

	r.reconcile(context.Background())

	want := []domain.PublishedServer{{Name: "crm", Generation: crmGeneration}, {Name: "local", Unmanaged: true}}
	if !slices.Equal(publisher.last.Servers, want) {
		t.Fatalf("Servers = %+v, want %+v", publisher.last.Servers, want)
	}
}

// Removed or switched off, a server leaves this worker with nothing new to
// connect — and that is still worth a publication, which is what lets the
// administration clear its tools without waiting for another change.
func TestReconciler_aServerTheConfigurationNoLongerWants_triggersAPublication(t *testing.T) {
	t.Parallel()
	r, publisher := connectedCRM(t, nil)

	r.reconcile(context.Background())

	if publisher.calls != 1 || slices.Contains(publisher.last.ServerNames(), "crm") {
		t.Fatalf("published %d times, last %+v; want one publication no longer speaking for crm",
			publisher.calls, publisher.last)
	}
}

// A server that stays down is tried on every pass. Nothing about the
// catalogue changed, so nothing is published: each publication takes the
// installation-wide catalogue lock, and one per refresh per replica for as
// long as a server is down is load for no news.
func TestReconciler_aServerThatStaysDown_publishesNothing(t *testing.T) {
	t.Parallel()
	r, publisher := reconcilerOver(t, []domain.MCPServer{crmServer()}, true)

	for range 3 {
		r.reconcile(context.Background())
	}

	if publisher.calls != 0 {
		t.Fatalf("published %d times while the only server stayed down, want none", publisher.calls)
	}
}

// A server still wanted but not reachable right now is not spoken for: this
// worker cannot see its tools, which says nothing about whether it offers
// them.
func TestReconciler_aWantedServerThatFailsToReconnect_isNotSpokenFor(t *testing.T) {
	t.Parallel()
	moved := crmServer()
	moved.URL = "https://tools.example.com/v2/mcp"
	moved.UpdatedAt = crmGeneration.Add(time.Hour)
	r, publisher := connectedCRM(t, []domain.MCPServer{moved})

	r.reconcile(context.Background())

	if slices.Contains(publisher.last.ServerNames(), "crm") {
		t.Fatalf("published %+v; want crm not spoken for", publisher.last)
	}
}
