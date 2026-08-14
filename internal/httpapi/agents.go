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

	// Whether it is running, which is a fact about the agent and not about
	// the version being read: an older version is shown beside the state of
	// the agent that has it, because there is only one thing to start.
	if s.pauses != nil {
		paused, err := s.pauses.IsPaused(ctx, wanted.ID)
		if err != nil {
			return nil, fmt.Errorf("agent state: %w", err)
		}
		wanted.Started = !paused
	}
	if s.promotions != nil {
		if wanted.Stage, err = s.promotions.StageOf(ctx, wanted.ID); err != nil {
			return nil, fmt.Errorf("agent stage: %w", err)
		}
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
	// The stages this version declares. Read separately from the summary for
	// the same reason the prose is: a page of twenty agents would otherwise
	// carry twenty processes nobody asked to read.
	if s.definitions != nil {
		declared, emits, err := s.definitions.Declared(ctx, wanted.ID, wanted.VersionID)
		if err != nil {
			return nil, fmt.Errorf("agent declarations: %w", err)
		}
		if len(declared) > 0 {
			out.Steps = ptr(stepsFrom(declared))
		}
		if len(emits) > 0 {
			out.Emits = &emits
		}
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

	// One read for the page, like the activity above. How far each agent is
	// trusted is the first thing somebody looks for on this screen now that a
	// draft cannot act at all.
	stages := map[domain.AgentID]domain.Stage{}
	if s.promotions != nil {
		if stages, err = s.promotions.Stages(ctx); err != nil {
			return nil, fmt.Errorf("agent stages: %w", err)
		}
	}

	// And whether each one is running, in one read for the same reason. An
	// agent with no row has never been decided about, which is stopped.
	stopped := map[domain.AgentID]bool{}
	if s.pauses != nil {
		if stopped, err = s.pauses.Paused(ctx); err != nil {
			return nil, fmt.Errorf("agent pauses: %w", err)
		}
	}

	// And which are out of circulation, so the listing can leave them out —
	// or answer with only those, for somebody who arrived from one of their
	// runs and needs to find the agent it belonged to.
	retired := map[domain.AgentID]bool{}
	if s.retirements != nil {
		if retired, err = s.retirements.Retired(ctx); err != nil {
			return nil, fmt.Errorf("retired agents: %w", err)
		}
	}
	wantRetired := req.Params.Retired != nil && *req.Params.Retired

	// An unscoped list is narrowed to what the caller may see rather than
	// refused: asking "which agents are there" should answer with theirs, not
	// with a permission error naming a scope they never mentioned (NF-06).
	visible := auth.VisibleScopes(ctx, domain.PermAgentRead)

	items := make([]openapi.Agent, 0, len(published))
	for _, a := range published {
		if !readable(a.Scope, visible) {
			continue
		}
		if retired[a.ID] != wantRetired {
			continue
		}
		a.Stage = stages[a.ID]
		paused, decided := stopped[a.ID]
		a.Started = decided && !paused
		a.Retired = retired[a.ID]
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
