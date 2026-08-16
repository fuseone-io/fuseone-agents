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
What a server offers is not what this installation brought in.

A server with two hundred tools is a Curator with two hundred decisions to
make, and most of them about tools nobody wants. So the surface is chosen per
server: of what you offer, these are the ones we take.

It is not a permission. A tool outside the surface is not "allowed with
conditions" or "blocked by policy" — it is not there. It reaches no model, it
answers no call, and the Gate is never asked about it, because the Gate decides
between an agent and a capability and this one is not a capability here.

Which also means a new tool appearing on a connected server arrives outside.
The same reason a new tool arrives unclassified: nobody has said.
*/

func offering(t *testing.T, surface *[]string, names ...string) *tools.Catalog {
	t.Helper()
	listed := make([]*mcp.Tool, 0, len(names))
	for _, name := range names {
		listed = append(listed, &mcp.Tool{
			Name: name, Description: name,
			InputSchema: map[string]any{"type": "object"},
		})
	}
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	err := catalog.AddServer(context.Background(), "crm",
		&fakeServer{list: listed, result: text("ok")}, surface)
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	return catalog
}

func names(chosen ...string) *[]string { return &chosen }

// The model is told about what this installation took, and nothing else. A
// schema for a tool that cannot be called is an invitation to call it.
func TestSchema_aToolOutsideTheSurface_isNotOfferedToTheModel(t *testing.T) {
	t.Parallel()
	catalog := offering(t, names("lookup"), "lookup", "delete_account")

	if _, _, _, ok := catalog.Schema("crm.lookup"); !ok {
		t.Error("the chosen tool is not offered")
	}
	if _, _, _, ok := catalog.Schema("crm.delete_account"); ok {
		t.Error("a tool outside the surface was described to the model")
	}
}

/*
And it cannot be called, whatever asked.

The model is not the only caller: a resumed run replays a call the ledger
holds, and a specification names tools by hand. A surface enforced only where
the schemas are written is a surface with a way round it.
*/
func TestInvoke_aToolOutsideTheSurface_isRefused(t *testing.T) {
	t.Parallel()
	catalog := offering(t, names("lookup"), "lookup", "delete_account")

	_, err := catalog.Invoke(context.Background(), engine.Call{Tool: "crm.delete_account"})
	if err == nil {
		t.Fatal("no error; a tool nobody brought in was called")
	}
}

/*
The Gate is never asked about it, because it is not a capability here.

Answering "unknown" rather than "refused" is the honest shape: the Gate decides
between an agent and something this installation has, and a tool outside the
surface is not something this installation has.
*/
func TestEffect_aToolOutsideTheSurface_isNotACapability(t *testing.T) {
	t.Parallel()
	catalog := offering(t, names("lookup"), "lookup", "delete_account")

	if _, known := catalog.Effect("crm.delete_account"); known {
		t.Error("the Gate was offered a tool that is not on the surface")
	}
}

/*
A ruling survives a tool leaving the surface.

Taking a tool out is a decision about scope, and it does not unmake the
judgement somebody wrote about what that tool does. Putting it back has to show
the earlier ruling — and let the digest decide whether it still stands, which
is the machinery this already has.
*/
func TestSync_aRulingForAToolOutsideTheSurface_isKeptAndNotOffered(t *testing.T) {
	t.Parallel()
	catalog := offering(t, names("lookup"), "lookup", "delete_account")

	if _, err := catalog.Sync(context.Background(), rulings{
		{Tool: "crm.delete_account", Effect: domain.EffectDestructive},
	}, domain.Scope{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Still not a capability, and the administration listing still knows what
	// was decided about it.
	if _, known := catalog.Effect("crm.delete_account"); known {
		t.Error("a ruling put an off-surface tool back within reach")
	}
	for _, e := range catalog.List() {
		if e.ID == "crm.delete_account" && e.Effect != domain.EffectDestructive {
			t.Errorf("effect = %v, want the ruling kept for when it returns", e.Effect)
		}
	}
}

/*
A server nobody has chosen a surface for offers everything it always did.

Absent is not empty. An installation upgrading has servers whose surface nobody
has ever picked, and reading that silence as "take nothing" would stop every
agent on the platform — the same shape as reading an absent digest as a
mismatch.
*/
func TestSchema_aServerWhoseSurfaceWasNeverChosen_offersWhatItAlwaysDid(t *testing.T) {
	t.Parallel()
	catalog := offering(t, nil, "lookup", "delete_account")

	for _, id := range []domain.ToolID{"crm.lookup", "crm.delete_account"} {
		if _, _, _, ok := catalog.Schema(id); !ok {
			t.Errorf("%s disappeared on upgrade", id)
		}
	}
}

// And a surface chosen as empty is a surface: this server, and none of it.
func TestSchema_aSurfaceChosenAsEmpty_takesNothing(t *testing.T) {
	t.Parallel()
	catalog := offering(t, names(), "lookup")

	if _, _, _, ok := catalog.Schema("crm.lookup"); ok {
		t.Error("a server with an empty surface still offered a tool")
	}
}
