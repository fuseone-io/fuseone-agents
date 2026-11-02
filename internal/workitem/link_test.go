package workitem_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/workitem"
)

/*
$fuseone.work_item.link through every door the claim's layer keeps (#204,
slice 2b): listed, described and classified by the platform, and never handed
to the layer below. A door that still compared against the claim alone would
let the link fall through to the MCP base — where a server registered under a
colliding name would answer for the platform, classify it a read, or run it.
*/

// The base the doors are checked against claims to know the link too, and
// says it is a read: a layer that delegated would be caught by either.
func TestLayer_describesTheLinkToTheModel(t *testing.T) {
	t.Parallel()
	l := workitem.NewLayer(&base{}, &base{})

	name, description, schema, ok := l.Schema(domain.ToolWorkItemLink)
	if !ok || name != string(domain.ToolWorkItemLink) || description == "from the server" {
		t.Fatalf("Schema = %q %q, %v; want the link described by the platform under its own id", name, description, ok)
	}
	if !strings.Contains(description, "create_pull_request") || !strings.Contains(description, "work_item_linked_elsewhere") {
		t.Fatalf("description %q does not tell the model when to link and what a refusal means", description)
	}
	required, _ := schema["required"].([]string)
	if !slices.Equal(required, []string{"ref"}) {
		t.Fatalf("schema %v must require ref and nothing else: the root is never the model's", schema)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, root := props["root"]; root || len(props) != 1 {
		t.Fatalf("schema properties %v; the run supplies the pull request alone", props)
	}
}

func TestLayer_classifiesTheLinkItself(t *testing.T) {
	t.Parallel()
	l := workitem.NewLayer(&base{}, &base{})

	if effect, ok := l.Effect(domain.ToolWorkItemLink); !ok || effect != domain.EffectWrite {
		t.Fatalf("Effect(link) = %s, %v; want write whatever the base says", effect, ok)
	}
	if _, ok := l.Dedupe(domain.ToolWorkItemLink); ok {
		t.Fatal("the link declared semantic dedupe; it is judged by the ledger every time")
	}
}

// The engine executes the link. Reserve and Invoke refuse it, and nothing of
// it reaches the base.
func TestLayer_neverForwardsTheLink(t *testing.T) {
	t.Parallel()
	below := &base{}
	l := workitem.NewLayer(below, below)
	call := engine.Call{Tool: domain.ToolWorkItemLink, Args: []byte(`{"ref":"github:fuseone-io/fuseone-agents#240"}`)}

	if err := l.Reserve(context.Background(), call); !errors.Is(err, workitem.ErrEngineExecuted) {
		t.Fatalf("Reserve(link) = %v, want ErrEngineExecuted", err)
	}
	if _, err := l.Invoke(context.Background(), call); !errors.Is(err, workitem.ErrEngineExecuted) {
		t.Fatalf("Invoke(link) = %v, want ErrEngineExecuted", err)
	}
	if len(below.invoked) != 0 {
		t.Fatalf("the link reached the layer below: %v", below.invoked)
	}
}

// Approval evidence and binding are the base's to give for its own tools; a
// link that parks for a person is the platform's, and the base is not asked.
func TestLayer_neverAsksTheBaseAboutTheLinksApproval(t *testing.T) {
	t.Parallel()
	below := &approvingBase{}
	l := workitem.NewLayer(below, below)
	call := engine.Call{Tool: domain.ToolWorkItemLink}

	if evidence, err := l.ApprovalEvidence(context.Background(), call); err != nil || !evidence.Empty() {
		t.Fatalf("ApprovalEvidence(link) = %+v, %v; want none", evidence, err)
	}
	if binding := l.ApprovalBinding(call); binding != "" {
		t.Fatalf("ApprovalBinding(link) = %q; want none", binding)
	}
	if below.asked != 0 {
		t.Fatalf("the base was asked %d times about the link's approval", below.asked)
	}
	if l.ApprovalBinding(engine.Call{Tool: "github.add_comment"}) != "bound" || below.asked != 1 {
		t.Fatal("another tool's approval binding was not passed down")
	}
}

type approvingBase struct {
	base
	asked int
}

func (b *approvingBase) ApprovalEvidence(context.Context, engine.Call) (domain.ApprovalEvidence, error) {
	b.asked++
	return domain.ApprovalEvidence{}, nil
}

func (b *approvingBase) ApprovalBinding(engine.Call) string {
	b.asked++
	return "bound"
}

// Listed once, as the platform's native write, and a base entry under the
// same id is dropped rather than shown beside it.
func TestToolList_listsTheLinkAndDropsACollidingBaseEntry(t *testing.T) {
	t.Parallel()
	entries, err := workitem.NewToolList(collidingSource{}).Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	var links []domain.ToolEntry
	for _, e := range entries {
		if e.ID == domain.ToolWorkItemLink {
			links = append(links, e)
		}
	}
	if len(links) != 1 || links[0].Effect != domain.EffectWrite || !links[0].Native || links[0].Server == "colliding" {
		t.Fatalf("link entries = %+v, want one native write", links)
	}
	if !slices.ContainsFunc(entries, func(e domain.ToolEntry) bool { return e.ID == domain.ToolWorkItemClaim }) {
		t.Fatalf("entries = %+v, the claim is gone", entries)
	}
}

type collidingSource struct{}

func (collidingSource) Tools(context.Context) ([]domain.ToolEntry, error) {
	return []domain.ToolEntry{
		{ID: domain.ToolWorkItemLink, Server: "colliding", Effect: domain.EffectRead},
		{ID: "github.add_comment", Server: "github", Effect: domain.EffectWrite},
	}, nil
}
