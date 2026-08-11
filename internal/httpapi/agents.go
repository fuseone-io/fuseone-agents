package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Agents is the registry of published versions, declared here by the consumer.
type Agents interface {
	List(ctx context.Context, scope domain.Scope, allVersions bool) ([]domain.AgentSummary, error)
}

func (s *Server) ListAgents(ctx context.Context, req openapi.ListAgentsRequestObject) (openapi.ListAgentsResponseObject, error) {
	scope := domain.Scope{}
	if req.Params.Company != nil {
		scope.Company = domain.CompanyID(*req.Params.Company)
	}
	if req.Params.Area != nil {
		scope.Area = domain.AreaID(*req.Params.Area)
	}

	// A named scope is checked in that scope. An author granted in cx must not
	// read marketing by asking for it, and must not be refused their own area
	// because they hold nothing in the installation's administrative scope.
	if scope.Company != "" || scope.Area != "" {
		if err := auth.Require(ctx, domain.PermAgentRead, scope); err != nil {
			return openapi.ListAgents403ApplicationProblemPlusJSONResponse{
				ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentRead, scope),
			}, nil
		}
	}
	if s.agents == nil {
		return openapi.ListAgents200JSONResponse{Items: []openapi.Agent{}}, nil
	}

	all := req.Params.AllVersions != nil && *req.Params.AllVersions
	published, err := s.agents.List(ctx, scope, all)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}

	// An unscoped list is narrowed to what the caller may see rather than
	// refused: asking "which agents are there" should answer with theirs, not
	// with a permission error naming a scope they never mentioned (NF-06).
	visible := auth.VisibleScopes(ctx, domain.PermAgentRead)

	items := make([]openapi.Agent, 0, len(published))
	for _, a := range published {
		if !readable(a.Scope, visible) {
			continue
		}
		items = append(items, agentFrom(a))
	}
	return openapi.ListAgents200JSONResponse{Items: items}, nil
}

// readable reports whether any scope the caller holds reaches this one.
//
// Contains, not equality: a company-wide grant reaches its areas, which is the
// whole point of the hierarchy — and comparing exactly made a curator granted
// across a company see nothing inside it.
func readable(scope domain.Scope, visible []domain.Scope) bool {
	for _, v := range visible {
		if v.Contains(scope) {
			return true
		}
	}
	return false
}

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
		PublishedAt: a.PublishedAt,
		Latest:      a.Latest,
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
