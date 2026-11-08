package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

// rediscoveryServers is the configuration plus what the administration asked
// every worker to do again, and what each worker answered.
type rediscoveryServers struct {
	probeServers
	requested map[string]int64
	receipts  []domain.MCPRediscoveryReceipt
}

func (s *rediscoveryServers) MCPRediscoveries(context.Context) (map[string]int64, error) {
	return s.requested, nil
}

func (s *rediscoveryServers) RecordMCPRediscovery(_ context.Context, r domain.MCPRediscoveryReceipt) error {
	s.receipts = append(s.receipts, r)
	return nil
}

// listingSession counts how often it is asked what it offers, and can be told
// to fail the next time.
type listingSession struct {
	testSession
	lists   int
	listErr error
}

func (s *listingSession) ListTools(ctx context.Context, p *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	s.lists++
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.testSession.ListTools(ctx, p)
}

type rediscoveryWorld struct {
	r        *reconciler
	catalog  *tools.Catalog
	servers  *rediscoveryServers
	session  *listingSession
	connects *int
}

func rediscoveryFixture(t *testing.T) rediscoveryWorld {
	t.Helper()
	server := domain.MCPServer{
		Name: "crm", Transport: domain.TransportHTTP, URL: "https://crm.example.com/mcp",
		Enabled: true, UpdatedAt: time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC),
	}
	w := rediscoveryWorld{
		catalog: tools.NewCatalog(engine.NewMemoryContent()),
		servers: &rediscoveryServers{
			probeServers: probeServers{servers: []domain.MCPServer{server}},
			requested:    map[string]int64{},
		},
		session:  &listingSession{testSession: testSession{tools: []*mcp.Tool{{Name: "note"}}}},
		connects: new(int),
	}
	w.r = newReconciler(w.catalog, w.servers, recordingHealth{}).
		publishingTo(&recordingPublisher{}).holdingAs("worker-a", nil)
	w.r.connectTo = func(
		ctx context.Context, catalog *tools.Catalog, server domain.MCPServer,
		_ domain.MCPCredentials, _ OAuthGrantStore, _ MCPUserCredentialStore, _ credentialPolicy,
		_ stdioEgressObserver,
	) error {
		*w.connects++
		return catalog.AddServer(ctx, server.Name, w.session, server.Surface)
	}
	return w
}

// A worker already connected when the request is made asks again once, on the
// session it has, and the new tool arrives unclassified.
func TestReconciler_aRediscoveryRequest_isAnsweredOnceOnTheOpenSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := rediscoveryFixture(t)
	w.r.reconcile(ctx)
	w.session.tools = append(w.session.tools, &mcp.Tool{Name: "close"})
	w.servers.requested["crm"] = 1

	w.r.reconcile(ctx)
	w.r.reconcile(ctx)

	if *w.connects != 1 || w.session.closed {
		t.Errorf("connects = %d, closed = %v; want the open session kept", *w.connects, w.session.closed)
	}
	if w.session.lists != 2 {
		t.Errorf("lists = %d, want one discovery and exactly one rediscovery", w.session.lists)
	}
	if effect, ok := w.catalog.Effect("crm.close"); !ok || effect != domain.EffectUnknown {
		t.Errorf("crm.close = %v/%v, want present and unclassified", effect, ok)
	}
	want := domain.MCPRediscoveryReceipt{Server: "crm", Worker: "worker-a", Generation: 1, Succeeded: true}
	if last := w.servers.receipts[len(w.servers.receipts)-1]; last != want {
		t.Errorf("last receipt = %+v, want %+v", last, want)
	}
}

// A worker that connects after the request already discovered what the server
// offers now. It answers the request with that discovery and owes nothing more.
func TestReconciler_aWorkerConnectingAfterTheRequest_answersItWithItsDiscovery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := rediscoveryFixture(t)
	w.servers.requested["crm"] = 3

	w.r.reconcile(ctx)
	w.r.reconcile(ctx)

	if w.session.lists != 1 {
		t.Errorf("lists = %d, want the connection's own discovery and nothing more", w.session.lists)
	}
	want := []domain.MCPRediscoveryReceipt{{Server: "crm", Worker: "worker-a", Generation: 3, Succeeded: true}}
	if len(w.servers.receipts) != 1 || w.servers.receipts[0] != want[0] {
		t.Errorf("receipts = %+v, want %+v", w.servers.receipts, want)
	}
}

// A rediscovery that fails leaves the session and the tools as they were, says
// so, and is not retried until somebody asks again.
func TestReconciler_aFailedRediscovery_keepsTheToolsAndIsNotRetried(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := rediscoveryFixture(t)
	w.r.reconcile(ctx)
	w.session.listErr = errors.New("upstream timed out")
	w.servers.requested["crm"] = 1

	w.r.reconcile(ctx)
	w.r.reconcile(ctx)

	if _, ok := w.catalog.Effect("crm.note"); !ok || w.session.closed || *w.connects != 1 {
		t.Errorf("note present = %v, closed = %v, connects = %d; want everything kept",
			ok, w.session.closed, *w.connects)
	}
	if w.session.lists != 2 {
		t.Errorf("lists = %d, want one failed attempt, not a retry every pass", w.session.lists)
	}
	want := domain.MCPRediscoveryReceipt{Server: "crm", Worker: "worker-a", Generation: 1, Succeeded: false}
	if last := w.servers.receipts[len(w.servers.receipts)-1]; last != want {
		t.Errorf("last receipt = %+v, want %+v", last, want)
	}
}

// A request for a server this worker does not hold — switched off, or never
// configured — asks nothing of it.
func TestReconciler_aRequestForAServerNotHeld_asksNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := rediscoveryFixture(t)
	w.r.reconcile(ctx)
	w.servers.servers[0].Enabled = false
	w.servers.requested["crm"] = 1
	w.servers.requested["billing"] = 1

	w.r.reconcile(ctx)

	if w.session.lists != 1 {
		t.Errorf("lists = %d, want no rediscovery of a server switched off", w.session.lists)
	}
	for _, r := range w.servers.receipts {
		if r.Generation == 1 {
			t.Errorf("receipt %+v answers a request for a server this worker does not hold", r)
		}
	}
}

// rulingsFor is the administration's record of what tools do.
type rulingsFor []domain.ToolClassification

func (r rulingsFor) List(context.Context, domain.Scope) ([]domain.ToolClassification, error) {
	return r, nil
}

// A tool that comes back by rediscovery finds the ruling already recorded for
// it, as it would on a reconnect.
func TestReconciler_aRediscoveredTool_takesItsRecordedRuling(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := rediscoveryFixture(t)
	w.r.reconcile(ctx)
	w.r.rulings = rulingsFor{{Tool: "crm.close", Effect: domain.EffectWrite}}
	w.session.tools = append(w.session.tools, &mcp.Tool{Name: "close"})
	w.servers.requested["crm"] = 1

	w.r.reconcile(ctx)

	if effect, _ := w.catalog.Effect("crm.close"); effect != domain.EffectWrite {
		t.Errorf("crm.close = %v, want the recorded write ruling applied", effect)
	}
}

// What a rediscovery found is published, so the screen where a Curator
// classifies tools lists the new one.
func TestReconciler_aRediscoveryThatFoundATool_publishesIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := rediscoveryFixture(t)
	publisher := &recordingPublisher{}
	w.r.publisher = publisher
	w.r.reconcile(ctx)
	w.session.tools = append(w.session.tools, &mcp.Tool{Name: "close"})
	w.servers.requested["crm"] = 1

	w.r.reconcile(ctx)

	found := false
	for _, e := range publisher.last.Entries {
		found = found || e.ID == "crm.close"
	}
	if publisher.calls != 2 || !found {
		t.Errorf("publications = %d, crm.close published = %v; want the rediscovery published",
			publisher.calls, found)
	}
}
