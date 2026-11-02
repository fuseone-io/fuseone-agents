// Package workitem describes the platform-owned work-item tools — the claim
// and the link — to the model and to the console, and keeps them out of every
// tool server's hands.
//
// It does not execute them. The engine does, against the ledger — only the
// run's single writer may append to it — so this layer answers the questions
// asked about the tools (what they look like, what effect they have) and
// refuses the one it must never be asked: to invoke them.
//
// Every door here asks Coordination(), the explicit and closed set, rather
// than comparing against one id: a door that knew only the claim would hand
// the link to the layer below, where a server registered under a colliding
// name would answer for the platform.
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

// claimDescription is what the model reads. It states the contract the ledger
// enforces, so a role learns from the tool itself to claim before it works and
// to move on when a card is taken.
const claimDescription = "Claim the work item (card) this run will work on, before doing any work on it. " +
	"A run holds one card until it ends: claiming the same card again succeeds, claiming another is refused. " +
	"Refused with work_item_held when another open run of this agent holds the card: pick the next card."

// linkDescription states the link's contract: the run names the pull request
// and nothing else, and when a refusal means the link is already there.
const linkDescription = "Link a pull request this run opened to the card this run holds, right after " +
	"create_pull_request succeeds, so the card's trail shows the runs that work on the pull request. " +
	"Give only the pull request's reference; the card is the one this run claimed. " +
	"Linking the same pull request again succeeds. Refused with work_item_linked_elsewhere when another " +
	"card owns the pull request, and with work_item_link_unclaimed when this run holds no card."

// ErrEngineExecuted is a work-item call that reached a tool layer. The engine
// executes it; forwarding it would let a server registered under a colliding
// name answer for the platform.
var ErrEngineExecuted = errors.New("workitem: the work-item tools are executed by the engine, never by a tool layer")

func schema(ref string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ref": map[string]any{
				"type":        "string",
				"maxLength":   domain.MaxWorkItemRefBytes,
				"description": ref,
			},
		},
		"required": []string{"ref"},
	}
}

// described is each work-item tool as the model reads it.
var described = map[domain.ToolID]struct{ description, ref string }{
	domain.ToolWorkItemClaim: {claimDescription, "The card's reference, exactly as the board names it: " +
		"github:owner/repo#123 for an issue or pull request, or ghproject:<node id> for a board item. An identifier only, never a description."},
	domain.ToolWorkItemLink: {linkDescription, "The pull request's reference, github:owner/repo#123, " +
		"in the same repository as the card this run holds. An identifier only, never a description."},
}

func entry(id domain.ToolID) domain.ToolEntry {
	return domain.ToolEntry{
		ID: id, Server: "fuseone:work_item",
		Description: described[id].description,
		Effect:      domain.EffectWrite,
		Native:      true,
		Scope:       domain.Scope{Company: domain.Installation},
		OnSurface:   true,
	}
}

// ToolEntry is the claim as the console's catalogue lists it.
func ToolEntry() domain.ToolEntry { return entry(domain.ToolWorkItemClaim) }

// LinkToolEntry is the link as the console's catalogue lists it.
func LinkToolEntry() domain.ToolEntry { return entry(domain.ToolWorkItemLink) }

// ToolSource is the catalogue the list extends.
type ToolSource interface {
	Tools(ctx context.Context) ([]domain.ToolEntry, error)
}

// ToolList adds the work-item tools beside an existing catalogue.
type ToolList struct{ base ToolSource }

func NewToolList(base ToolSource) *ToolList { return &ToolList{base: base} }

func (l *ToolList) Tools(ctx context.Context) ([]domain.ToolEntry, error) {
	var out []domain.ToolEntry
	if l != nil && l.base != nil {
		base, err := l.base.Tools(ctx)
		if err != nil {
			return nil, err
		}
		for _, e := range base {
			if !e.ID.Coordination() {
				out = append(out, e)
			}
		}
	}
	out = append(out, ToolEntry(), LinkToolEntry())
	slices.SortFunc(out, func(a, b domain.ToolEntry) int {
		return strings.Compare(string(a.ID), string(b.ID))
	})
	return out, nil
}

// Layer answers for the work-item tools and passes everything else down.
type Layer struct {
	base    engine.Tools
	catalog engine.Catalog
}

func NewLayer(base engine.Tools, catalog engine.Catalog) *Layer {
	return &Layer{base: base, catalog: catalog}
}

// Effect classifies the work-item tools as writes whatever the layer below
// says: their answers depend on every other open run, so they are never reads.
func (l *Layer) Effect(id domain.ToolID) (domain.Effect, bool) {
	if id.Coordination() {
		return domain.EffectWrite, true
	}
	if l.catalog == nil {
		return domain.EffectUnknown, false
	}
	return l.catalog.Effect(id)
}

// Dedupe never declares one for a work-item tool: the ledger judges every
// attempt.
func (l *Layer) Dedupe(id domain.ToolID) (domain.ToolDedupe, bool) {
	if id.Coordination() || l.catalog == nil {
		return domain.ToolDedupe{}, false
	}
	return l.catalog.Dedupe(id)
}

func (l *Layer) Schema(id domain.ToolID) (string, string, map[string]any, bool) {
	if d, ok := described[id]; ok && id.Coordination() {
		return string(id), d.description, schema(d.ref), true
	}
	if id.Coordination() {
		// A coordination id with no description here is described by nobody:
		// the layer below must not describe the platform's tools.
		return "", "", nil, false
	}
	if schemas, ok := l.catalog.(engine.Schemas); ok {
		return schemas.Schema(id)
	}
	return "", "", nil, false
}

func (l *Layer) Reserve(ctx context.Context, call engine.Call) error {
	if call.Tool.Coordination() {
		return fmt.Errorf("reserve %s: %w", call.Tool, ErrEngineExecuted)
	}
	if l.base == nil {
		return fmt.Errorf("workitem: no tool layer for %s", call.Tool)
	}
	return l.base.Reserve(ctx, call)
}

func (l *Layer) Invoke(ctx context.Context, call engine.Call) (engine.ToolResult, error) {
	if call.Tool.Coordination() {
		return engine.ToolResult{}, fmt.Errorf("invoke %s: %w", call.Tool, ErrEngineExecuted)
	}
	if l.base == nil {
		return engine.ToolResult{}, fmt.Errorf("workitem: no tool layer for %s", call.Tool)
	}
	return l.base.Invoke(ctx, call)
}

// ApprovalEvidence and ApprovalBinding pass the question down, as every layer
// must: the engine asks the outermost one. A work-item tool is the platform's,
// and the layer below is never asked about it.
func (l *Layer) ApprovalEvidence(ctx context.Context, call engine.Call) (domain.ApprovalEvidence, error) {
	if provider, ok := l.base.(engine.ApprovalEvidencer); ok && !call.Tool.Coordination() {
		return provider.ApprovalEvidence(ctx, call)
	}
	return domain.ApprovalEvidence{}, nil
}

func (l *Layer) ApprovalBinding(call engine.Call) string {
	if binder, ok := l.base.(engine.ApprovalBinder); ok && !call.Tool.Coordination() {
		return binder.ApprovalBinding(call)
	}
	return ""
}
