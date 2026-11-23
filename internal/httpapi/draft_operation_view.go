package httpapi

import (
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

// operationView is an operation as the wire carries it: the inverse of
// operationOf, block operations included, so a proposal reads as exactly
// what accepting it would apply.
func operationView(op draft.Operation) openapi.DraftOperation {
	out := openapi.DraftOperation{Kind: openapi.DraftOperationKind(op.Kind())}
	if fill, ok := operationViews[op.Kind()]; ok {
		fill(op, &out)
	}
	return out
}

var operationViews = map[string]func(draft.Operation, *openapi.DraftOperation){
	"set_identity": func(op draft.Operation, out *openapi.DraftOperation) {
		o := asValue[draft.SetIdentity](op)
		out.Name, out.Company, out.Area = ptr(o.Name), ptr(string(o.Scope.Company)), ptr(string(o.Scope.Area))
	},
	"set_standing_approvals": func(op draft.Operation, out *openapi.DraftOperation) {
		out.StandingApprovals = standingFrom(asValue[draft.SetStandingApprovals](op).Policy)
	},
	"set_model": func(op draft.Operation, out *openapi.DraftOperation) {
		o := asValue[draft.SetModel](op)
		out.Provider, out.Model, out.Effort = ptr(o.Provider), ptr(o.Model), ptr(o.Effort)
	},
	"set_fallback": func(op draft.Operation, out *openapi.DraftOperation) {
		out.Fallback = pairFrom(asValue[draft.SetFallback](op).Pair)
	},
	"set_cache_prefix_ttl": func(op draft.Operation, out *openapi.DraftOperation) {
		out.CachePrefixTtl = ptr(asValue[draft.SetCachePrefixTTL](op).TTL)
	},
	"set_reply_locale": func(op draft.Operation, out *openapi.DraftOperation) {
		out.ReplyLocale = ptr(asValue[draft.SetReplyLocale](op).Locale)
	},
	"add_tool": func(op draft.Operation, out *openapi.DraftOperation) {
		out.Tool = ptr(string(asValue[draft.AddTool](op).Tool))
	},
	"remove_tool": func(op draft.Operation, out *openapi.DraftOperation) {
		out.Tool = ptr(string(asValue[draft.RemoveTool](op).Tool))
	},
	"set_budget": func(op draft.Operation, out *openapi.DraftOperation) {
		b := asValue[draft.SetBudget](op).Budget
		out.Budget = &openapi.Budget{Micros: ptr(b.Micros), Tokens: ptr(b.Tokens), ToolCalls: ptr(b.ToolCalls),
			Steps: ptr(b.Steps), WallClockMs: ptr(b.WallClockMS)}
	},
	"set_triggers": func(op draft.Operation, out *openapi.DraftOperation) {
		out.Triggers = ptr(triggersView(asValue[draft.SetTriggers](op).Triggers))
	},
	"set_memory_learning": func(op draft.Operation, out *openapi.DraftOperation) {
		out.MemoryLearning = memoryLearningFrom(asValue[draft.SetMemoryLearning](op).Policy)
	},
	"set_approval_delivery": func(op draft.Operation, out *openapi.DraftOperation) {
		out.Approvals = approvalsFrom(asValue[draft.SetApprovalDelivery](op).Policy)
	},
	"set_emits": func(op draft.Operation, out *openapi.DraftOperation) {
		out.Emits = ptr(eventsFrom(asValue[draft.SetEmits](op).Emits))
	},
	"add_step": func(op draft.Operation, out *openapi.DraftOperation) {
		out.Step = ptr(stepsFrom([]spec.Step{asValue[draft.AddStep](op).Step})[0])
	},
	"edit_step": func(op draft.Operation, out *openapi.DraftOperation) {
		o := asValue[draft.EditStep](op)
		out.StepName, out.Step = ptr(o.Name), ptr(stepsFrom([]spec.Step{o.Step})[0])
	},
	"remove_step": func(op draft.Operation, out *openapi.DraftOperation) {
		out.StepName = ptr(asValue[draft.RemoveStep](op).Name)
	},
	"insert_block": func(op draft.Operation, out *openapi.DraftOperation) {
		o := asValue[draft.InsertBlock](op)
		out.At, out.Text, out.Locale = ptr(o.At), ptr(o.Text), ptr(o.Locale)
		out.BlockKind = ptr(openapi.DraftOperationBlockKind(o.Type))
	},
	"replace_block": func(op draft.Operation, out *openapi.DraftOperation) {
		o := asValue[draft.ReplaceBlock](op)
		out.BlockId, out.Text, out.Locale = ptr(o.ID), ptr(o.Text), ptr(o.Locale)
	},
	"edit_block": func(op draft.Operation, out *openapi.DraftOperation) {
		o := asValue[draft.EditBlock](op)
		out.BlockId, out.OldText, out.Text = ptr(o.ID), ptr(o.Old), ptr(o.New)
	},
	"remove_block": func(op draft.Operation, out *openapi.DraftOperation) {
		out.BlockId = ptr(asValue[draft.RemoveBlock](op).ID)
	},
	"move_block": func(op draft.Operation, out *openapi.DraftOperation) {
		o := asValue[draft.MoveBlock](op)
		out.BlockId, out.To = ptr(o.ID), ptr(o.To)
	},
}

// asValue reads an operation as its value type, whether it was handed over
// by value or by pointer.
func asValue[T draft.Operation](op draft.Operation) T {
	if v, ok := op.(T); ok {
		return v
	}
	var zero T
	if pointer, ok := any(op).(*T); ok && pointer != nil {
		return *pointer
	}
	return zero
}

func triggersView(triggers []spec.Trigger) []openapi.AgentTrigger {
	out := make([]openapi.AgentTrigger, 0, len(triggers))
	for _, t := range triggers {
		out = append(out, apiTrigger(domain.AgentTrigger{
			Type: t.Type, Schedule: t.Schedule, Path: t.Path, Event: t.Event, From: t.From, Events: t.Events,
		}))
	}
	return out
}
