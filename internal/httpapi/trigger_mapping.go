package httpapi

import (
	"slices"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

// One mapping each way, so the field a webhook needs cannot reach the API in
// one direction and die in the other: read back without its source, a GitHub
// hook would be republished as an ordinary one by the next save of the form.

// apiTrigger is a published trigger as the console reads it.
func apiTrigger(t domain.AgentTrigger) openapi.AgentTrigger {
	out := openapi.AgentTrigger{Type: openapi.AgentTriggerType(t.Type)}
	if t.Schedule != "" {
		out.Schedule = ptr(t.Schedule)
	}
	if t.Path != "" {
		out.Path = ptr(t.Path)
	}
	if t.Event != "" {
		out.Event = ptr(t.Event)
	}
	if t.From != "" {
		from := openapi.AgentTriggerFrom(t.From)
		out.From = &from
	}
	if len(t.Events) > 0 {
		out.Events = ptr(slices.Clone(t.Events))
	}
	return out
}

// specTrigger is a trigger from the form, as the file it renders declares it.
func specTrigger(t openapi.AgentTrigger) spec.Trigger {
	out := spec.Trigger{
		Type: string(t.Type), Schedule: valueOr(t.Schedule),
		Path: valueOr(t.Path), Event: valueOr(t.Event),
		Events: slices.Clone(valueOr(t.Events)),
	}
	if t.From != nil {
		out.From = string(*t.From)
	}
	return out
}
