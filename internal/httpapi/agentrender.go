package httpapi

import (
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Turning a published version into what a screen reads.
//
// Separate from the routes because it is the one part with no decisions in
// it: every field here is a rename, and the moment one of them starts
// choosing what to show it belongs with the route that chose.

func agentFrom(a domain.AgentSummary) openapi.Agent {
	tools := make([]string, 0, len(a.Tools))
	for _, t := range a.Tools {
		tools = append(tools, string(t))
	}

	agent := openapi.Agent{
		AgentId:   string(a.ID),
		VersionId: string(a.VersionID),
		Scope:     openapi.Scope{Company: string(a.Scope.Company), Area: string(a.Scope.Area)},
		Name:      a.Name,
		Provider:  a.Provider,
		Model:     a.Model,
		Tools:     tools,
		Budget: openapi.Budget{
			Micros: ptr(a.Budget.Micros), Tokens: ptr(a.Budget.Tokens),
			ToolCalls: ptr(a.Budget.ToolCalls), Steps: ptr(a.Budget.Steps),
			WallClockMs: ptr(a.Budget.WallClockMS),
		},
		MemoryLearning: memoryLearningFrom(a.MemoryLearning),
		PublishedAt:    a.PublishedAt,
		Latest:         a.Latest,
	}
	agent.Stage = ptr(openapi.Stage(domain.StageOf(string(a.Stage))))
	agent.Paused = ptr(!a.Started)
	if a.Retired {
		agent.Retired = ptr(true)
	}
	if a.Effort != "" {
		agent.Effort = ptr(a.Effort)
	}
	if a.PublishedBy != "" {
		agent.PublishedBy = ptr(string(a.PublishedBy))
	}
	if len(a.Triggers) > 0 {
		triggers := make([]openapi.AgentTrigger, 0, len(a.Triggers))
		for _, t := range a.Triggers {
			trigger := openapi.AgentTrigger{Type: openapi.AgentTriggerType(t.Type)}
			if t.Schedule != "" {
				trigger.Schedule = ptr(t.Schedule)
			}
			if t.Path != "" {
				trigger.Path = ptr(t.Path)
			}
			if t.Event != "" {
				trigger.Event = ptr(t.Event)
			}
			triggers = append(triggers, trigger)
		}
		agent.Triggers = &triggers
	}
	return agent
}

func memoryLearningFrom(p domain.MemoryLearningPolicy) *openapi.MemoryLearningPolicy {
	normalized := p.Normalize()
	if !normalized.Enabled() {
		return nil
	}
	mode := openapi.MemoryLearningMode(normalized.Mode)
	return &openapi.MemoryLearningPolicy{
		Mode: &mode, MinObservations: ptr(normalized.MinObservations),
		TtlDays: ptr(normalized.TTLDays),
	}
}

func activityFrom(a domain.AgentActivity) openapi.AgentActivity {
	out := openapi.AgentActivity{
		Runs: a.Runs, Finished: a.Finished, Waiting: a.Waiting, CostMicros: a.CostMicros,
	}
	if a.LastPhase != "" {
		out.LastPhase = ptr(openapi.Phase(a.LastPhase))
	}
	if !a.LastRunAt.IsZero() {
		out.LastRunAt = ptr(a.LastRunAt)
	}
	return out
}
