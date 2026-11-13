package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/admin"
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

/*
The worker opens the credential of the row it read (NT-019).

It names the server as it read it, where it is stored and at which revision, so
the store never hands over the credential of a server of the name stored
elsewhere since. A row that changed is read again on the next pass, and is not
a failure of the server's.
*/
func TestReconciler_opensTheCredentialOfTheRowItRead(t *testing.T) {
	t.Parallel()
	w := rediscoveryFixture(t)
	w.servers.servers[0].Scope = cxArea

	w.r.reconcile(context.Background())

	opened := w.servers.opened
	if len(opened) != 1 || opened[0].Scope != cxArea || !opened[0].UpdatedAt.Equal(w.servers.servers[0].UpdatedAt) {
		t.Errorf("opened = %+v, want the server as read: at %s, its revision", opened, cxArea)
	}
}

func TestReconciler_aServerChangedSinceItWasRead_isNotConnected(t *testing.T) {
	t.Parallel()
	w := rediscoveryFixture(t)
	w.servers.credentialsErr = fmt.Errorf("wrapped: %w", admin.ErrMCPServerChanged)
	health := recordingHealth{}
	w.r.health = health

	w.r.reconcile(context.Background())

	if *w.connects != 0 {
		t.Errorf("connects = %d, want none with a credential of another row", *w.connects)
	}
	if h, seen := health["crm"]; seen {
		t.Errorf("health = %+v, want nothing recorded: the server did not fail, it changed", h)
	}
}

// scopedPersonal holds one person's credential for the server row it was
// stored for, as the administration does, and records the row it was asked of.
type scopedPersonal struct {
	at    domain.Scope
	creds domain.MCPCredentials
	asked []domain.Scope
}

func (p *scopedPersonal) MCPPersonalCredential(
	_ context.Context, at domain.Scope, _ string, _ domain.UserID,
) (domain.MCPCredentials, bool, error) {
	p.asked = append(p.asked, at)
	if at != p.at {
		return domain.MCPCredentials{}, false, nil
	}
	return p.creds, true, nil
}

var _ MCPUserCredentialStore = (*scopedPersonal)(nil)

/*
A person's credential travels only to the server row it was stored for.

Ana stored hers for jira at acme/finance. The worker holds a server of that
name read at acme/cx: it asks for the credential of the row it connected, and
the request goes out with the server's own credential, never Ana's.
*/
func TestAuthenticatedClient_asksForThePersonalCredentialOfTheRowItConnected(t *testing.T) {
	t.Parallel()
	var seen string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(remote.Close)
	personal := &scopedPersonal{
		at: domain.Scope{Company: "acme", Area: "finance"}, creds: domain.MCPCredentials{Token: "ana-finance"},
	}
	client, err := authenticatedClientWithPolicy(domain.MCPServer{Name: "jira", Scope: cxArea},
		domain.MCPCredentials{Token: "cx-shared"}, nil, personal, credentialPolicy{})
	if err != nil {
		t.Fatalf("authenticatedClientWithPolicy: %v", err)
	}
	req, err := http.NewRequestWithContext(tools.WithCaller(t.Context(), "usr_ana"), http.MethodGet, remote.URL, nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()

	if seen != "Bearer cx-shared" || len(personal.asked) != 1 || personal.asked[0] != cxArea {
		t.Errorf("Authorization = %q, asked %v; want the cx server's own token, asked of %s", seen, personal.asked, cxArea)
	}
}

// A refreshed grant is written back to the row the worker connected, named by
// where it is stored, so it cannot land on a server of the name stored
// elsewhere since.
func TestOAuthRefresh_isWrittenBackToTheRowItConnected(t *testing.T) {
	t.Parallel()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fresh", "expires_in": 3600})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(remote.Close)
	grant := domain.MCPOAuthGrant{
		AccessToken: "expired", RefreshToken: "refresh", TokenURL: remote.URL + "/token",
		ExpiresAtUnix: time.Now().Add(-time.Minute).Unix(),
	}
	store := &recordingOAuthStore{}
	client, err := authenticatedClientWithPolicy(domain.MCPServer{
		Name: "jira", Scope: cxArea, Egress: &domain.MCPEgress{Origins: []string{mcpOrigin(t, remote.URL)}},
	}, domain.MCPCredentials{OAuth: &grant}, store, nil, credentialPolicy{})
	if err != nil {
		t.Fatalf("authenticatedClientWithPolicy: %v", err)
	}
	resp, err := client.Get(remote.URL + "/resource")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = resp.Body.Close()

	if store.name != "jira" || store.scope != cxArea {
		t.Errorf("written back to %s at %s, want jira at %s", store.name, store.scope, cxArea)
	}
}
