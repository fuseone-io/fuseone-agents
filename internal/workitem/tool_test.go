package workitem_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/workitem"
)

// base is the layer underneath: it classifies, describes and invokes one MCP
// tool, and — the trap — also claims to know the claim, so a layer that
// delegated would hand the platform's id to a server.
type base struct{ invoked []domain.ToolID }

func (b *base) Effect(id domain.ToolID) (domain.Effect, bool) {
	switch id {
	case "github.add_comment":
		return domain.EffectWrite, true
	case domain.ToolWorkItemClaim:
		return domain.EffectRead, true
	}
	return domain.EffectUnknown, false
}

func (b *base) Dedupe(id domain.ToolID) (domain.ToolDedupe, bool) {
	return domain.ToolDedupe{WindowSeconds: 60, ArgPaths: []string{"ref"}}, true
}

func (b *base) Schema(id domain.ToolID) (string, string, map[string]any, bool) {
	return string(id), "from the server", map[string]any{"type": "object"}, true
}

func (b *base) Reserve(context.Context, engine.Call) error { return nil }

func (b *base) Invoke(_ context.Context, call engine.Call) (engine.ToolResult, error) {
	b.invoked = append(b.invoked, call.Tool)
	return engine.ToolResult{}, nil
}

func TestLayer_describesTheClaimToTheModel(t *testing.T) {
	t.Parallel()
	l := workitem.NewLayer(&base{}, &base{})

	name, description, schema, ok := l.Schema(domain.ToolWorkItemClaim)
	if !ok || name != string(domain.ToolWorkItemClaim) {
		t.Fatalf("Schema = %q, %v; want the claim described under its own id", name, ok)
	}
	if !strings.Contains(description, "one card") {
		t.Fatalf("description %q does not tell the model a run holds one card", description)
	}
	required, _ := schema["required"].([]string)
	if !slices.Contains(required, "ref") {
		t.Fatalf("schema %v does not require ref", schema)
	}
	if got, _, _, _ := l.Schema("github.add_comment"); got != "github.add_comment" {
		t.Fatalf("another tool's schema was not passed down: %q", got)
	}
}

// The platform classifies its own tool: a write, never deduplicated, whatever
// the layer below would say about a colliding name.
func TestLayer_classifiesTheClaimItself(t *testing.T) {
	t.Parallel()
	l := workitem.NewLayer(&base{}, &base{})

	if effect, ok := l.Effect(domain.ToolWorkItemClaim); !ok || effect != domain.EffectWrite {
		t.Fatalf("Effect(claim) = %s, %v; want write", effect, ok)
	}
	if _, ok := l.Dedupe(domain.ToolWorkItemClaim); ok {
		t.Fatal("the claim declared semantic dedupe; it is judged by the ledger every time")
	}
	if effect, _ := l.Effect("github.add_comment"); effect != domain.EffectWrite {
		t.Fatalf("another tool's effect was not passed down: %s", effect)
	}
}

// The engine executes the claim. Reaching a tool layer at all is a wiring
// mistake, refused here rather than forwarded to a server.
func TestLayer_neverForwardsTheClaim(t *testing.T) {
	t.Parallel()
	below := &base{}
	l := workitem.NewLayer(below, below)

	if _, err := l.Invoke(context.Background(), engine.Call{Tool: domain.ToolWorkItemClaim}); err == nil {
		t.Fatal("Invoke(claim) succeeded; only the engine may execute it")
	}
	if len(below.invoked) != 0 {
		t.Fatalf("the claim reached the layer below: %v", below.invoked)
	}
	if _, err := l.Invoke(context.Background(), engine.Call{Tool: "github.add_comment"}); err != nil {
		t.Fatalf("another tool was not passed down: %v", err)
	}
}

func TestToolList_listsTheClaimBesideTheBase(t *testing.T) {
	t.Parallel()
	entries, err := workitem.NewToolList(nil).Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	i := slices.IndexFunc(entries, func(e domain.ToolEntry) bool { return e.ID == domain.ToolWorkItemClaim })
	if i < 0 || entries[i].Effect != domain.EffectWrite || !entries[i].Native {
		t.Fatalf("entries = %+v, want the claim as a native write", entries)
	}
}
