package httpapi

import (
	"errors"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

// renderAndParse turns the form's fields into the file, and the file into the
// specification. Both directions, because the file is the artefact and the
// version is its digest.
func renderAndParse(id string, in openapi.AgentDefinition) ([]byte, spec.Spec, error) {
	if id == "" {
		return nil, spec.Spec{}, errors.New("um agente precisa de um identificador")
	}

	draft := spec.Spec{
		ID: domain.AgentID(id), Name: in.Name,
		Company: domain.CompanyID(in.Company), Area: domain.AreaID(in.Area),
		Provider: in.Provider, Model: in.Model, Effort: valueOr(in.Effort),
		Fallback:       pairOf(in.Fallback),
		CachePrefixTTL: valueOr(in.CachePrefixTtl),
		ReplyLocale:    valueOr(in.ReplyLocale),
		Instructions:   in.Instructions,
	}
	for _, t := range valueOr(in.Tools) {
		draft.Tools = append(draft.Tools, domain.ToolID(t))
	}
	draft.Emits = emitsOf(in.Emits)
	if in.Budget != nil {
		draft.Budget = domain.Budget{
			Micros: valueOr(in.Budget.Micros), Tokens: valueOr(in.Budget.Tokens),
			ToolCalls: valueOr(in.Budget.ToolCalls), Steps: valueOr(in.Budget.Steps),
			WallClockMS: valueOr(in.Budget.WallClockMs),
		}
	}
	draft.MemoryLearning = memoryLearningOf(in.MemoryLearning)
	draft.Approvals = approvalsOf(in.Approvals)
	draft.StandingApprovals = standingOf(in.StandingApprovals)
	for _, t := range valueOr(in.Triggers) {
		draft.Triggers = append(draft.Triggers, specTrigger(t))
	}
	draft.Steps = stepsOf(in.Steps)

	rendered, err := spec.Render(draft)
	if err != nil {
		return nil, spec.Spec{}, err
	}
	// Parsed back rather than trusted: validation lives in the parser, and a
	// console that skipped it would accept definitions a file could not.
	parsed, err := spec.Parse("console", rendered)
	if err != nil {
		return nil, spec.Spec{}, err
	}
	return rendered, parsed, nil
}

func memoryLearningOf(in *openapi.MemoryLearningPolicy) domain.MemoryLearningPolicy {
	if in == nil || in.Mode == nil {
		return domain.MemoryLearningPolicy{Mode: domain.MemoryLearningOff}
	}
	return domain.MemoryLearningPolicy{
		Mode:            domain.MemoryLearningMode(*in.Mode),
		MinObservations: valueOr(in.MinObservations),
		TTLDays:         valueOr(in.TtlDays),
	}.Normalize()
}

/*
approvalsOf is what the client sent back, on the way in.

Absent is the console alone. There is no "leave what is stored" here, because
publishing writes a whole version: a client that omits this is publishing an
agent that asked for nothing, which is why the read hands it back and the
console carries it through untouched.
*/
func approvalsOf(in *openapi.ApprovalPolicy) domain.ApprovalPolicy {
	if in == nil {
		return domain.ApprovalPolicy{}
	}
	out := domain.ApprovalPolicy{Direct: valueOr(in.Direct)}
	for _, one := range valueOrSlice(in.Notify) {
		out.Notify = append(out.Notify, domain.UserID(one))
	}
	return out.Normalize()
}

func emitsOf(in *[]openapi.AgentEvent) spec.Emits {
	if in == nil {
		return nil
	}
	out := make(spec.Emits, 0, len(*in))
	for _, event := range *in {
		next := domain.AgentEvent{
			Event:   event.Event,
			Context: valueOr(event.Context),
		}
		if event.Artifacts != nil {
			next.Artifacts = append([]string(nil), *event.Artifacts...)
		}
		out = append(out, next)
	}
	return out
}

/*
stepsOf carries the declared steps into the specification.

They used to be dropped here, which was not a missing feature but a quiet
widening: `reaches` is what the Gate allows while a run sits at a step, so a
definition rendered without them hands every step the whole capability pack —
on an edit somebody made for an unrelated reason, with nothing on screen
saying so.
*/
func stepsOf(in *[]openapi.AgentStep) []spec.Step {
	if in == nil {
		return nil
	}
	out := make([]spec.Step, 0, len(*in))
	for _, step := range *in {
		next := spec.Step{
			Name:      step.Name,
			StopsWhen: valueOr(step.StopsWhen),
			Model:     valueOr(step.Model),
			Effort:    valueOr(step.Effort),
		}
		for _, tool := range valueOr(step.Reaches) {
			next.Reaches = append(next.Reaches, domain.ToolID(tool))
		}
		out = append(out, next)
	}
	return out
}
