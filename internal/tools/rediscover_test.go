package tools_test

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

// A server that gained a tool while connected is asked again on the session
// already open: nothing is reconnected, and the new tool arrives as any
// discovered tool does — unclassified.
func TestRediscover_aToolAddedWhileConnected_arrivesUnclassifiedOnTheSameSession(t *testing.T) {
	t.Parallel()
	srv := noteServer()
	c, _ := catalogWith(t, srv)
	srv.list = append(srv.list, &mcp.Tool{Name: "close", Description: "Close a case."})

	if err := c.Rediscover(t.Context(), "crm", nil); err != nil {
		t.Fatalf("Rediscover: %v", err)
	}

	if effect, ok := c.Effect("crm.close"); !ok || effect != domain.EffectUnknown {
		t.Errorf("crm.close = %v/%v, want present and unclassified", effect, ok)
	}
	if srv.closed {
		t.Error("the session was closed; rediscovery must ask on the one already open")
	}
	if _, err := c.Invoke(t.Context(), engine.Call{RunID: "run-1", Seq: 1, Tool: "crm.note"}); err != nil {
		t.Errorf("a call after rediscovery failed: %v", err)
	}
}

// A tool whose definition did not change keeps the ruling it already has, so a
// call between the rediscovery and the next sync is not refused as unknown.
func TestRediscover_anUnchangedTool_keepsItsRuling(t *testing.T) {
	t.Parallel()
	srv := noteServer()
	c, _ := catalogWith(t, srv)
	if _, err := c.Sync(t.Context(), staticRulings{{Tool: "crm.note", Effect: domain.EffectWrite}}, domain.Scope{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if err := c.Rediscover(t.Context(), "crm", nil); err != nil {
		t.Fatalf("Rediscover: %v", err)
	}

	if effect, _ := c.Effect("crm.note"); effect != domain.EffectWrite {
		t.Errorf("crm.note = %v after rediscovery, want the write ruling kept", effect)
	}
}

// A tool whose definition changed under the same name is a tool nobody has
// judged, and rediscovery treats it as one.
func TestRediscover_aChangedDefinition_losesItsRuling(t *testing.T) {
	t.Parallel()
	srv := noteServer()
	c, _ := catalogWith(t, srv)
	if _, err := c.Sync(t.Context(), staticRulings{{Tool: "crm.note", Effect: domain.EffectRead}}, domain.Scope{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	srv.list[0].Description = "Record a note, and email it to the customer."

	if err := c.Rediscover(t.Context(), "crm", nil); err != nil {
		t.Fatalf("Rediscover: %v", err)
	}

	if effect, _ := c.Effect("crm.note"); effect != domain.EffectUnknown {
		t.Errorf("crm.note = %v after its definition changed, want unclassified", effect)
	}
}

// A tool the server stopped offering leaves the catalogue.
func TestRediscover_aToolNoLongerOffered_isDropped(t *testing.T) {
	t.Parallel()
	srv := noteServer()
	srv.list = append(srv.list, &mcp.Tool{Name: "close"})
	c, _ := catalogWith(t, srv)
	srv.list = srv.list[:1]

	if err := c.Rediscover(t.Context(), "crm", nil); err != nil {
		t.Fatalf("Rediscover: %v", err)
	}

	if _, ok := c.Effect("crm.close"); ok {
		t.Error("crm.close is still offered after the server stopped listing it")
	}
}

// A rediscovery that fails changes nothing: the session and the tools the
// catalogue had stay exactly as they were.
func TestRediscover_aFailedListing_leavesSessionAndToolsAlone(t *testing.T) {
	t.Parallel()
	srv := noteServer()
	c, _ := catalogWith(t, srv)
	if _, err := c.Sync(t.Context(), staticRulings{{Tool: "crm.note", Effect: domain.EffectWrite}}, domain.Scope{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	srv.listErr = errors.New("upstream timed out")

	if err := c.Rediscover(t.Context(), "crm", nil); err == nil {
		t.Fatal("Rediscover succeeded over a listing that failed")
	}

	if effect, _ := c.Effect("crm.note"); effect != domain.EffectWrite {
		t.Errorf("crm.note = %v after a failed rediscovery, want it untouched", effect)
	}
	if srv.closed || !c.Connected("crm") {
		t.Error("a failed rediscovery touched the session")
	}
}

// A server that is not connected here has nothing to ask again.
func TestRediscover_aServerNotConnected_isRefused(t *testing.T) {
	t.Parallel()
	c, _ := catalogWith(t, noteServer())

	if err := c.Rediscover(t.Context(), "billing", nil); err == nil {
		t.Fatal("Rediscover of a server never connected succeeded")
	}
}

// A tool that arrives by rediscovery carries what the server's registration
// established, even when the server offered nothing before.
func TestRediscover_aNewTool_carriesTheRegistrationsProvenance(t *testing.T) {
	t.Parallel()
	srv := &fakeServer{}
	c := tools.NewCatalog(engine.NewMemoryContent())
	if err := c.AddServer(t.Context(), "github", srv, nil,
		tools.WithProvenance(domain.ProvenanceHostedGitHubApp)); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	srv.list = []*mcp.Tool{{Name: "create_pull_request"}}

	if err := c.Rediscover(t.Context(), "github", nil); err != nil {
		t.Fatalf("Rediscover: %v", err)
	}

	entry, ok := c.Lookup("github.create_pull_request")
	if !ok || entry.Provenance != domain.ProvenanceHostedGitHubApp {
		t.Errorf("entry = %+v/%v, want the registration's provenance", entry.Provenance, ok)
	}
}

// reconnectingDuringList stands in for a session that the reconciler replaced
// while it was still answering a rediscovery.
type reconnectingDuringList struct {
	fakeServer
	replace func()
}

func (r *reconnectingDuringList) ListTools(ctx context.Context, p *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	if r.replace != nil {
		r.replace()
	}
	return r.fakeServer.ListTools(ctx, p)
}

// A rediscovery answered by a session that was replaced meanwhile describes a
// server that is no longer the one connected, and is not applied over the
// replacement's own discovery.
func TestRediscover_aSessionReplacedWhileListing_isNotApplied(t *testing.T) {
	t.Parallel()
	c := tools.NewCatalog(engine.NewMemoryContent())
	stale := &reconnectingDuringList{fakeServer: fakeServer{list: []*mcp.Tool{{Name: "old"}}}}
	if err := c.AddServer(t.Context(), "crm", stale, nil); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	stale.replace = func() {
		_ = c.AddServer(t.Context(), "crm", &fakeServer{list: []*mcp.Tool{{Name: "new"}}}, nil)
	}

	err := c.Rediscover(t.Context(), "crm", nil)

	if !errors.Is(err, tools.ErrSessionReplaced) {
		t.Fatalf("err = %v, want ErrSessionReplaced", err)
	}
	if _, ok := c.Lookup("crm.new"); !ok {
		t.Error("the replacement's discovery was overwritten by the stale listing")
	}
}
