package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/catalogue"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
ListTemplates is what an author can start from (PRD FU-16).

Readable by anyone who can read agents. A template is shipped with the product
and holds nothing about this installation — no tools, no scope, no data — so
there is nothing here to narrow by scope.
*/
func (s *Server) ListTemplates(_ context.Context, _ openapi.ListTemplatesRequestObject) (openapi.ListTemplatesResponseObject, error) {
	all, err := catalogue.All()
	if err != nil {
		return nil, fmt.Errorf("read the template catalogue: %w", err)
	}

	items := make([]openapi.AgentTemplate, 0, len(all))
	for _, template := range all {
		items = append(items, templateFrom(template))
	}
	return openapi.ListTemplates200JSONResponse{Items: items}, nil
}

func templateFrom(t catalogue.Template) openapi.AgentTemplate {
	out := openapi.AgentTemplate{
		Id: t.ID, Name: t.Name, Summary: t.Summary,
		Instructions: t.Instructions, Needs: t.Needs,
	}
	if t.Area != "" {
		out.Area = ptr(t.Area)
	}
	if b := t.Budget; b != (catalogue.Budget{}) {
		out.Budget = &openapi.Budget{
			Micros: ptr(b.Micros), Tokens: ptr(b.Tokens),
			ToolCalls: ptr(b.ToolCalls), Steps: ptr(b.Steps),
			WallClockMs: ptr(b.WallClockMS),
		}
	}
	if len(t.Triggers) > 0 {
		triggers := make([]openapi.AgentTrigger, 0, len(t.Triggers))
		for _, trigger := range t.Triggers {
			out := openapi.AgentTrigger{Type: openapi.AgentTriggerType(trigger.Type)}
			if trigger.Schedule != "" {
				out.Schedule = ptr(trigger.Schedule)
			}
			if trigger.Path != "" {
				out.Path = ptr(trigger.Path)
			}
			if trigger.Event != "" {
				out.Event = ptr(trigger.Event)
			}
			triggers = append(triggers, out)
		}
		out.Triggers = &triggers
	}
	return out
}
