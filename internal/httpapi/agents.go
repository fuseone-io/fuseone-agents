package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Agents is the registry of published versions, declared here by the consumer.
//
// Instructions is separate from the listing because the text is only ever
// wanted one version at a time: a page of twenty agents would otherwise carry
// twenty bodies of prose nobody asked to read.
type Agents interface {
	List(ctx context.Context, scope domain.Scope, allVersions bool) ([]domain.AgentSummary, error)
	Versions(ctx context.Context, agent domain.AgentID) ([]domain.AgentSummary, error)
	Instructions(ctx context.Context, agent domain.AgentID, version domain.VersionID) (text, source string, err error)
}

// GetAgent reads one published version, exactly as it was published.
//
// Every failure answers 404 — no such agent, no such version, an area the
// caller cannot see. Distinguishing them would confirm that an agent exists
// in an area somebody has no business knowing about.
func (s *Server) GetAgent(ctx context.Context, req openapi.GetAgentRequestObject) (openapi.GetAgentResponseObject, error) {
	absent := openapi.GetAgent404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}
	if s.agents == nil {
		return absent, nil
	}

	versions, err := s.agents.Versions(ctx, domain.AgentID(req.AgentId))
	if err != nil {
		return nil, fmt.Errorf("agent versions: %w", err)
	}
	if len(versions) == 0 {
		return absent, nil
	}
	if !readable(versions[0].Scope, auth.VisibleScopes(ctx, domain.PermAgentRead)) {
		return absent, nil
	}

	// The newest unless one is named. A run pinned to an older version is
	// explained by that version, never by whatever was published since.
	wanted := versions[0]
	if req.Params.Version != nil && *req.Params.Version != "" {
		found := false
		for _, v := range versions {
			if string(v.VersionID) == *req.Params.Version {
				wanted, found = v, true
				break
			}
		}
		if !found {
			return absent, nil
		}
	}

	text, source, err := s.agents.Instructions(ctx, wanted.ID, wanted.VersionID)
	if err != nil {
		return nil, fmt.Errorf("agent instructions: %w", err)
	}

	out := openapi.AgentDetail{
		Agent:    agentFrom(wanted),
		Versions: make([]openapi.AgentVersion, 0, len(versions)),
	}
	if text != "" {
		out.Instructions = ptr(text)
	}
	if source != "" {
		out.Source = ptr(source)
	}
	for _, v := range versions {
		version := openapi.AgentVersion{
			VersionId: string(v.VersionID), PublishedAt: v.PublishedAt, Latest: v.Latest,
		}
		if v.PublishedBy != "" {
			version.PublishedBy = ptr(string(v.PublishedBy))
		}
		out.Versions = append(out.Versions, version)
	}

	if activity, err := s.activityOf(ctx, wanted); err != nil {
		return nil, err
	} else if activity != nil {
		out.Agent.Activity = activity
	}
	return openapi.GetAgent200JSONResponse(out), nil
}

// activityOf reads how the agent has been doing, or nil when it never ran.
func (s *Server) activityOf(ctx context.Context, a domain.AgentSummary) (*openapi.AgentActivity, error) {
	seen, err := s.store.AgentActivity(ctx, domain.RunFilter{Scope: a.Scope, AgentID: a.ID})
	if err != nil {
		return nil, fmt.Errorf("agent activity: %w", err)
	}
	for _, activity := range seen {
		if activity.AgentID == a.ID {
			return ptr(activityFrom(activity)), nil
		}
	}
	return nil, nil
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

	// One grouped read for every agent on the page rather than one per agent.
	activity, err := s.store.AgentActivity(ctx, domain.RunFilter{Scope: scope})
	if err != nil {
		return nil, fmt.Errorf("agent activity: %w", err)
	}
	byAgent := make(map[domain.AgentID]domain.AgentActivity, len(activity))
	for _, a := range activity {
		byAgent[a.AgentID] = a
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
		agent := agentFrom(a)
		if seen, ran := byAgent[a.ID]; ran {
			agent.Activity = ptr(activityFrom(seen))
		}
		items = append(items, agent)
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
