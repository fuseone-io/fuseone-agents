package workitem_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/workitem"
)

// linkTool is the id the writer will register in #204, slice 2b. Spelled out
// here rather than named in the domain: on this release nothing may hold it.
const linkTool = domain.ToolID("$fuseone.work_item.link")

/*
The link's readers ship one release ahead of its writer (#204, slice 2a), and
the writer is the tool. Were it callable here, a worker on this release could
seal links while older workers still fail on them — the exact upgrade the split
exists to avoid. So on this release the id is nothing: not the platform's, not
a coordination the Gate waves through, not listed to the console or the model,
and not described or classified by the layer that owns the claim.

When 2b registers the tool, this test is the one it turns around.
*/
func TestWorkItemLinkTool_notExposedBeforeItsReadersShip(t *testing.T) {
	t.Parallel()
	anchorEveryDoorOnTheClaim(t)

	if linkTool.PlatformOwned() {
		t.Errorf("%s: PlatformOwned = true; the writer is not on this release", linkTool)
	}
	if linkTool.Coordination() {
		t.Errorf("%s: Coordination = true; the Gate would let it through without approval", linkTool)
	}

	tools, err := workitem.NewToolList(nil).Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	for _, entry := range tools {
		if entry.ID == linkTool {
			t.Errorf("the catalogue lists %s: %+v", linkTool, entry)
		}
	}

	layer := workitem.NewLayer(nil, nil)
	if effect, ok := layer.Effect(linkTool); ok {
		t.Errorf("the layer classifies %s as %s; it must not know it", linkTool, effect)
	}
	if name, _, _, ok := layer.Schema(linkTool); ok {
		t.Errorf("the layer describes %s to the model as %q", linkTool, name)
	}
}

// The anchor: every door the link is held out of is open to the claim, so the
// absence above is the link's and not a door that no longer opens for anyone.
func anchorEveryDoorOnTheClaim(t *testing.T) {
	t.Helper()
	claim := domain.ToolWorkItemClaim
	tools, err := workitem.NewToolList(nil).Tools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].ID != claim {
		t.Fatalf("Tools = %+v, %v; want the claim listed", tools, err)
	}
	layer := workitem.NewLayer(nil, nil)
	_, known := layer.Effect(claim)
	_, _, _, described := layer.Schema(claim)
	if !claim.PlatformOwned() || !claim.Coordination() || !known || !described {
		t.Fatalf("the claim is not exposed through every door the link is checked at")
	}
}
