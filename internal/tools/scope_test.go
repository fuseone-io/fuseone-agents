package tools_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

/*
The catalogue knows where each server belongs (NT-019, phase 2 readers).

A server's tools are its scope's: an area's server reaches that area's runs and
no other, and the Gate reads it from here. A server added with no scope, as a
--mcp flag adds one, is the installation's.
*/

var (
	cx           = domain.Scope{Company: "acme", Area: "cx"}
	installation = domain.Scope{Company: domain.Installation}
)

func scopedLookupServer() *fakeServer {
	return &fakeServer{list: []*mcp.Tool{{
		Name: "lookup", Description: "Look a customer up",
		InputSchema: map[string]any{"type": "object"},
	}}}
}

func TestScope_aServerAddedWithAScope_isThatScopes(t *testing.T) {
	t.Parallel()
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	if err := catalog.AddServer(context.Background(), "crm", scopedLookupServer(), nil, tools.WithScope(cx)); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	if got, ok := catalog.Scope("crm.lookup"); !ok || got != cx {
		t.Errorf("scope = %s (%v), want %s", got, ok, cx)
	}
}

func TestScope_aServerAddedWithNoScope_isTheInstallations(t *testing.T) {
	t.Parallel()
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	if err := catalog.AddServer(context.Background(), "crm", scopedLookupServer(), nil); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	if got, ok := catalog.Scope("crm.lookup"); !ok || got != installation {
		t.Errorf("scope = %s (%v), want the installation's", got, ok)
	}
}

/*
A tool has at most two rulings: its owner's, stored at the server's scope, and
the platform's, stored at the installation, which wins (NT-019). A ruling at
any other scope is nobody's word about this tool.
*/
func TestSync_thePlatformsRuling_winsOverTheOwners(t *testing.T) {
	t.Parallel()
	owner := domain.ToolClassification{Tool: "crm.lookup", Effect: domain.EffectRead, Scope: cx}
	platform := domain.ToolClassification{Tool: "crm.lookup", Effect: domain.EffectWrite, Scope: installation}
	elsewhere := domain.ToolClassification{Tool: "crm.lookup", Effect: domain.EffectRead,
		Scope: domain.Scope{Company: "acme", Area: "finance"}}

	for _, one := range []struct {
		name   string
		given  rulings
		effect domain.Effect
	}{
		{"owner alone", rulings{owner}, domain.EffectRead},
		{"platform over owner", rulings{owner, platform}, domain.EffectWrite},
		{"platform over owner, either order", rulings{platform, owner}, domain.EffectWrite},
		{"a ruling of another scope", rulings{elsewhere}, domain.EffectUnknown},
	} {
		fresh := tools.NewCatalog(engine.NewMemoryContent())
		if err := fresh.AddServer(context.Background(), "crm", scopedLookupServer(), nil, tools.WithScope(cx)); err != nil {
			t.Fatalf("AddServer: %v", err)
		}
		if _, err := fresh.Sync(context.Background(), one.given, domain.Scope{}); err != nil {
			t.Fatalf("%s: Sync: %v", one.name, err)
		}
		if effect, _ := fresh.Effect("crm.lookup"); effect != one.effect {
			t.Errorf("%s: effect = %v, want %v", one.name, effect, one.effect)
		}
	}
}

// A stale ceiling stops the tool. It never hands the decision back to the owner.
func TestSync_aStalePlatformRuling_doesNotFallBackToTheOwners(t *testing.T) {
	t.Parallel()
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	if err := catalog.AddServer(context.Background(), "crm", scopedLookupServer(), nil, tools.WithScope(cx)); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	given := rulings{
		{Tool: "crm.lookup", Effect: domain.EffectRead, Scope: cx},
		{Tool: "crm.lookup", Effect: domain.EffectWrite, Scope: installation, Digest: "sha256:an-older-definition"},
	}
	if _, err := catalog.Sync(context.Background(), given, domain.Scope{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if effect, _ := catalog.Effect("crm.lookup"); effect == domain.EffectRead {
		t.Error("a stale platform ruling fell back to the owner's")
	}
}

/*
A stale ceiling arriving after the owner's ruling took effect stops the tool.

In sequence, on one catalogue: the owner's read applies, then a later sweep
brings the platform's ruling, judged against an older definition. Marking the
entry stale and keeping what the owner had set left the tool readable, the
fall-back NT-019 rules out. A stale decision returns the tool to how discovery
leaves it: no effect, untrusted, nothing to compensate with, no dedupe.
*/
func TestSync_aStaleCeilingAfterTheOwnersRuling_returnsTheToolToUnclassified(t *testing.T) {
	t.Parallel()
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	if err := catalog.AddServer(context.Background(), "crm", scopedLookupServer(), nil, tools.WithScope(cx)); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	owner := domain.ToolClassification{
		Tool: "crm.lookup", Effect: domain.EffectRead, Scope: cx, CompensatedBy: "crm.undo",
		Dedupe: domain.ToolDedupe{WindowSeconds: 60, ArgPaths: []string{"id"}},
	}
	if _, err := catalog.Sync(context.Background(), rulings{owner}, domain.Scope{}); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if effect, _ := catalog.Effect("crm.lookup"); effect != domain.EffectRead {
		t.Fatalf("after the owner's ruling, effect = %v, want read", effect)
	}

	stale := domain.ToolClassification{Tool: "crm.lookup", Effect: domain.EffectWrite,
		Scope: installation, Digest: "sha256:an-older-definition"}
	if _, err := catalog.Sync(context.Background(), rulings{owner, stale}, domain.Scope{}); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	entry, ok := catalog.Lookup("crm.lookup")
	if !ok {
		t.Fatal("crm.lookup left the catalogue")
	}
	if entry.Effect != domain.EffectUnknown || !entry.Untrusted || entry.CompensatedBy != "" || entry.Dedupe.Enabled() {
		t.Errorf("entry = effect %v untrusted %v compensatedBy %q dedupe %+v, want unclassified",
			entry.Effect, entry.Untrusted, entry.CompensatedBy, entry.Dedupe)
	}
	if !entry.Stale {
		t.Error("the entry does not say its ruling is stale")
	}
}
