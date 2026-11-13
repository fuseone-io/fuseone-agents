package main

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

/*
A request asked of one server is not answered by another of its name (NT-019).

Asked of the server stored at acme/cx, a probe or a rediscovery names that row.
When the worker reads it, the server it holds under the name may be stored
elsewhere: removed and stored again meanwhile. That is another server, and the
request is not run against it, nor answered by connecting it.
*/

var cxArea = domain.Scope{Company: "acme", Area: "cx"}

func TestConsumeProbes_aProbeForAServerStoredElsewhereNow_isNotAnswered(t *testing.T) {
	ctx := t.Context()
	finance := domain.Scope{Company: "acme", Area: "finance"}
	server := domain.MCPServer{
		Name: "jira", Transport: domain.TransportHTTP, URL: "https://tools.example.com/mcp",
		Enabled: true, Scope: finance,
	}
	configured := &probeServers{
		servers: []domain.MCPServer{server}, probes: []string{"jira"},
		probeScopes: map[string]domain.Scope{"jira": cxArea},
	}
	r := newReconciler(tools.NewCatalog(engine.NewMemoryContent()), configured, recordingHealth{})
	connects := 0
	r.connectTo = func(
		context.Context, *tools.Catalog, domain.MCPServer,
		domain.MCPCredentials, OAuthGrantStore, MCPUserCredentialStore, credentialPolicy,
		stdioEgressObserver,
	) error {
		connects++
		return nil
	}

	if r.consumeProbes(ctx, map[string]domain.MCPServer{"jira": server}) {
		t.Error("the probe changed the catalogue")
	}
	if connects != 0 {
		t.Errorf("the worker connected %d times for a probe asked of another server", connects)
	}
}

func TestReconciler_aRediscoveryForAServerStoredElsewhereNow_isNotAnswered(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := rediscoveryFixture(t)
	w.r.reconcile(ctx)
	w.session.tools = append(w.session.tools, &mcp.Tool{Name: "close"})
	// The fixture's server is the installation's; the request was asked of a
	// server of this name stored at acme/cx, which is gone.
	w.servers.requested["crm"] = 1
	w.servers.requestScopes = map[string]domain.Scope{"crm": cxArea}

	w.r.reconcile(ctx)

	if w.session.lists != 1 {
		t.Errorf("lists = %d, want no rediscovery for a request about another server", w.session.lists)
	}
	if _, ok := w.catalog.Effect("crm.close"); ok {
		t.Error("crm.close arrived: the held server answered a request asked of another")
	}
}

// Connecting a server answers what had been asked of it, and only of it: a
// request about a server of the name stored elsewhere is not answered by this
// one's connection.
func TestReconciler_connectingAServer_doesNotAnswerARequestAboutAnother(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := rediscoveryFixture(t)
	w.servers.requested["crm"] = 1
	w.servers.requestScopes = map[string]domain.Scope{"crm": cxArea}

	w.r.reconcile(ctx)

	if *w.connects != 1 {
		t.Fatalf("connects = %d, want the installation's server connected", *w.connects)
	}
	for _, receipt := range w.servers.receipts {
		if receipt.Generation != 0 {
			t.Errorf("receipt = %+v, want generation 1 unanswered: it was asked of the server at %s", receipt, cxArea)
		}
	}
}

// A receipt says where the server it answers for is stored, so the status of
// a server stored there later does not show it.
func TestReconciler_aReceipt_namesTheRowItAnswered(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := rediscoveryFixture(t)
	w.servers.servers[0].Scope = cxArea
	w.servers.requested["crm"] = 1
	w.servers.requestScopes = map[string]domain.Scope{"crm": cxArea}

	w.r.reconcile(ctx)

	if len(w.servers.receipts) != 1 || w.servers.receipts[0].Scope != cxArea {
		t.Errorf("receipts = %+v, want one about the server at %s", w.servers.receipts, cxArea)
	}
}
