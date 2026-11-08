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

// flakyPublisher refuses the first publications it is handed.
type flakyPublisher struct {
	recordingPublisher
	fails int
}

func (f *flakyPublisher) Publish(ctx context.Context, pub domain.ToolPublication) error {
	if f.fails > 0 {
		f.fails--
		return errors.New("database unavailable")
	}
	return f.recordingPublisher.Publish(ctx, pub)
}

// A publication the administration did not take is sent again on the next
// pass, though nothing else changed. Without this the worker offered a tool
// that no screen listed until some other change came along.
func TestReconciler_aPublicationThatFailed_isSentAgainOnTheNextPass(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "crm", Transport: domain.TransportHTTP, URL: "https://crm.example.com/mcp",
		Enabled: true, UpdatedAt: time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC),
	}
	publisher := &flakyPublisher{fails: 1}
	r := newReconciler(tools.NewCatalog(engine.NewMemoryContent()),
		&probeServers{servers: []domain.MCPServer{server}}, recordingHealth{}).publishingTo(publisher)
	r.connectTo = func(
		ctx context.Context, catalog *tools.Catalog, server domain.MCPServer,
		_ domain.MCPCredentials, _ OAuthGrantStore, _ MCPUserCredentialStore, _ credentialPolicy,
		_ stdioEgressObserver,
	) error {
		return catalog.AddServer(ctx, server.Name, &testSession{tools: []*mcp.Tool{{Name: "note"}}}, nil)
	}

	r.reconcile(ctx) // connects, and the publication is refused
	r.reconcile(ctx) // nothing changed, and the publication goes again
	r.reconcile(ctx) // nothing changed, and nothing is owed

	if publisher.calls != 1 || len(publisher.last.Entries) != 1 {
		t.Errorf("publications taken = %d with %d entries; want the refused one sent once more, and no more",
			publisher.calls, len(publisher.last.Entries))
	}
}
