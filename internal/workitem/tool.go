// Package workitem describes the platform-owned work-item claim to the model
// and to the console, and keeps it out of every tool server's hands.
//
// It does not execute the claim. The engine does, against the ledger — only
// the run's single writer may append to it — so this layer answers the
// questions asked about the tool (what it looks like, what effect it has) and
// refuses the one it must never be asked: to invoke it.
package workitem

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// description is what the model reads. It states the contract the ledger
// enforces, so a role learns from the tool itself to claim before it works and
// to move on when a card is taken.
const description = "Claim the work item (card) this run will work on, before doing any work on it. " +
	"A run holds one card until it ends: claiming the same card again succeeds, claiming another is refused. " +
	"Refused with work_item_held when another open run of this agent holds the card: pick the next card."

// ErrEngineExecuted is a claim that reached a tool layer. The engine executes
// it; forwarding it would let a server registered under a colliding name
// answer for the platform.
var ErrEngineExecuted = errors.New("workitem: the claim is executed by the engine, never by a tool layer")

func schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ref": map[string]any{
				"type":      "string",
				"maxLength": domain.MaxWorkItemRefBytes,
				"description": "The card's reference, exactly as the board names it: " +
					"github:owner/repo#123 for an issue or pull request, or ghproject:<node id> for a board item. An identifier only, never a description.",
			},
		},
		"required": []string{"ref"},
	}
}

// ToolEntry is the claim as the console's catalogue lists it.
func ToolEntry() domain.ToolEntry {
	return domain.ToolEntry{
		ID: domain.ToolWorkItemClaim, Server: "fuseone:work_item",
		Description: description,
		Effect:      domain.EffectWrite,
		Native:      true,
		Scope:       domain.Scope{Company: domain.Installation},
		OnSurface:   true,
	}
}

// ToolSource is the catalogue the list extends.
type ToolSource interface {
	Tools(ctx context.Context) ([]domain.ToolEntry, error)
}

// ToolList adds the claim beside an existing catalogue.
type ToolList struct{ base ToolSource }

func NewToolList(base ToolSource) *ToolList { return &ToolList{base: base} }

func (l *ToolList) Tools(ctx context.Context) ([]domain.ToolEntry, error) {
	var out []domain.ToolEntry
	if l != nil && l.base != nil {
		base, err := l.base.Tools(ctx)
		if err != nil {
			return nil, err
		}
		for _, entry := range base {
			if entry.ID != domain.ToolWorkItemClaim {
				out = append(out, entry)
			}
		}
	}
	out = append(out, ToolEntry())
	slices.SortFunc(out, func(a, b domain.ToolEntry) int {
		return strings.Compare(string(a.ID), string(b.ID))
	})
	return out, nil
}

// Layer answers for the claim and passes everything else down.
type Layer struct {
	base    engine.Tools
	catalog engine.Catalog
}

func NewLayer(base engine.Tools, catalog engine.Catalog) *Layer {
	return &Layer{base: base, catalog: catalog}
}

// Effect classifies the claim as a write whatever the layer below says: its
// answer depends on every other open run, so it is never a read.
func (l *Layer) Effect(id domain.ToolID) (domain.Effect, bool) {
	if id == domain.ToolWorkItemClaim {
		return domain.EffectWrite, true
	}
	if l.catalog == nil {
		return domain.EffectUnknown, false
	}
	return l.catalog.Effect(id)
}

// Dedupe never declares one for the claim: the ledger judges every attempt.
func (l *Layer) Dedupe(id domain.ToolID) (domain.ToolDedupe, bool) {
	if id == domain.ToolWorkItemClaim || l.catalog == nil {
		return domain.ToolDedupe{}, false
	}
	return l.catalog.Dedupe(id)
}

func (l *Layer) Schema(id domain.ToolID) (string, string, map[string]any, bool) {
	if id == domain.ToolWorkItemClaim {
		return string(id), description, schema(), true
	}
	if schemas, ok := l.catalog.(engine.Schemas); ok {
		return schemas.Schema(id)
	}
	return "", "", nil, false
}

func (l *Layer) Reserve(ctx context.Context, call engine.Call) error {
	if call.Tool == domain.ToolWorkItemClaim {
		return fmt.Errorf("reserve: %w", ErrEngineExecuted)
	}
	if l.base == nil {
		return fmt.Errorf("workitem: no tool layer for %s", call.Tool)
	}
	return l.base.Reserve(ctx, call)
}

func (l *Layer) Invoke(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	if call.Tool == domain.ToolWorkItemClaim {
		return engine.ToolResult{}, fmt.Errorf("invoke: %w", ErrEngineExecuted)
	}
	if l.base == nil {
		return engine.ToolResult{}, fmt.Errorf("workitem: no tool layer for %s", call.Tool)
	}
	return l.base.Invoke(ctx, call)
}

// ApprovalEvidence and ApprovalBinding pass the question down, as every layer
// must: the engine asks the outermost one. The claim never needs either.
func (l *Layer) ApprovalEvidence(ctx context.Context, call engine.Call) (domain.ApprovalEvidence, error) {
	if provider, ok := l.base.(engine.ApprovalEvidencer); ok && call.Tool != domain.ToolWorkItemClaim {
		return provider.ApprovalEvidence(ctx, call)
	}
	return domain.ApprovalEvidence{}, nil
}

func (l *Layer) ApprovalBinding(call engine.Call) string {
	if binder, ok := l.base.(engine.ApprovalBinder); ok && call.Tool != domain.ToolWorkItemClaim {
		return binder.ApprovalBinding(call)
	}
	return ""
}
