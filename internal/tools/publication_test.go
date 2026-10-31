package tools_test

import (
	"context"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

// A server that answered with no tools has still answered: the publication
// names it, so its earlier tools are dropped rather than left on offer. A
// server that was removed is no longer this catalogue's to speak for.
func TestPublication_namesEveryConnectedServer_evenOneOfferingNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := tools.NewCatalog(engine.NewMemoryContent())
	t.Cleanup(func() { _ = c.Close() })
	for name, list := range map[string][]*mcp.Tool{
		"crm":    {{Name: "lookup"}},
		"github": nil,
		"gone":   {{Name: "anything"}},
	} {
		if err := c.AddServer(ctx, name, &fakeServer{list: list}, nil); err != nil {
			t.Fatalf("AddServer %s: %v", name, err)
		}
	}
	if err := c.RemoveServer("gone"); err != nil {
		t.Fatalf("RemoveServer: %v", err)
	}

	pub := c.Publication()

	if !slices.Equal(pub.Servers, []string{"crm", "github"}) {
		t.Fatalf("Servers = %v, want the two connected, including the one offering nothing", pub.Servers)
	}
	if len(pub.Entries) != 1 || pub.Entries[0].ID != "crm.lookup" {
		t.Fatalf("Entries = %+v, want crm.lookup alone", pub.Entries)
	}
}
