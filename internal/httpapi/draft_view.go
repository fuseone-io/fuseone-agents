package httpapi

import (
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

// draftView is a draft as the workbench reads it: the definition with its
// instructions rendered, the blocks a proposal names (never their raw bytes,
// which are the codec's), and what publication still needs.
func draftView(o drafts.Opened) openapi.AgentDraft {
	out := openapi.AgentDraft{
		AgentId: string(o.Agent), DraftId: o.DraftID,
		Definition: definitionOf(o.Draft.Definition()),
		Blocks:     make([]openapi.DraftBlock, 0, len(o.Draft.Blocks)),
		Missing:    make([]openapi.AgentDraftMissing, 0, len(o.Missing)),
	}
	if o.BaseVersion != "" {
		out.BaseVersionId = ptr(string(o.BaseVersion))
	}
	for _, b := range o.Draft.Blocks {
		block := openapi.DraftBlock{Id: b.ID, Kind: openapi.DraftBlockKind(b.Kind), Text: b.Text}
		if b.Label != "" {
			block.Label = ptr(b.Label)
		}
		out.Blocks = append(out.Blocks, block)
	}
	for _, m := range o.Missing {
		out.Missing = append(out.Missing, openapi.AgentDraftMissing(m))
	}
	return out
}

// definitionOf is a specification as the API's fields, the inverse of
// draftOfFields: what a client reads back is what it would send.
func definitionOf(s spec.Spec) openapi.AgentDefinition {
	tools := make([]string, 0, len(s.Tools))
	for _, t := range s.Tools {
		tools = append(tools, string(t))
	}
	out := openapi.AgentDefinition{
		Name: s.Name, Company: string(s.Company), Area: string(s.Area),
		Provider: s.Provider, Model: s.Model, Instructions: s.Instructions,
		Tools: &tools, Fallback: pairFrom(s.Fallback),
		Budget: &openapi.Budget{
			Micros: ptr(s.Budget.Micros), Tokens: ptr(s.Budget.Tokens), ToolCalls: ptr(s.Budget.ToolCalls),
			Steps: ptr(s.Budget.Steps), WallClockMs: ptr(s.Budget.WallClockMS),
		},
		MemoryLearning:    memoryLearningFrom(s.MemoryLearning),
		Approvals:         approvalsFrom(s.Approvals),
		StandingApprovals: standingFrom(s.StandingApprovals),
		Emits:             ptr(eventsFrom(s.Emits)),
		Steps:             ptr(stepsFrom(s.Steps)),
	}
	if s.Effort != "" {
		out.Effort = ptr(s.Effort)
	}
	if s.CachePrefixTTL != "" {
		out.CachePrefixTtl = ptr(s.CachePrefixTTL)
	}
	if s.ReplyLocale != "" {
		out.ReplyLocale = ptr(s.ReplyLocale)
	}
	triggers := make([]openapi.AgentTrigger, 0, len(s.Triggers))
	for _, t := range s.Triggers {
		triggers = append(triggers, apiTrigger(domain.AgentTrigger{
			Type: t.Type, Schedule: t.Schedule, Path: t.Path, Event: t.Event, From: t.From, Events: t.Events,
		}))
	}
	out.Triggers = &triggers
	return out
}
