package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Promotions moves an agent between stages, declared here by the consumer.
type Promotions interface {
	Stages(ctx context.Context) (map[domain.AgentID]domain.Stage, error)
	StageOf(ctx context.Context, agent domain.AgentID) (domain.Stage, error)
	SetStage(ctx context.Context, agent domain.AgentID, stage domain.Stage, by domain.UserID) error
}

// WithPromotions wires how far each agent is trusted, and who may change it.
func (s *Server) WithPromotions(p Promotions) *Server {
	s.promotions = p
	return s
}

// SetAgentStage promotes or demotes an agent.
func (s *Server) SetAgentStage(
	ctx context.Context, req openapi.SetAgentStageRequestObject,
) (openapi.SetAgentStageResponseObject, error) {
	scope, ok := s.agentScope(ctx, domain.AgentID(req.AgentId))
	if !ok || s.promotions == nil || req.Body == nil {
		return openapi.SetAgentStage404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
		}, nil
	}

	// Promoting is causing every run this agent makes from now on that nobody
	// will be asked about, so it takes the authority to cause runs rather than
	// the one to write definitions.
	if err := auth.Require(ctx, domain.PermRunTrigger, scope); err != nil {
		return openapi.SetAgentStage403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermRunTrigger, scope),
		}, nil
	}

	stage := domain.Stage(req.Body.Stage)
	if !stage.Valid() {
		return openapi.SetAgentStage400ApplicationProblemPlusJSONResponse(problem(
			http.StatusBadRequest, "Not a stage", fmt.Sprintf("%q is not a stage", stage))), nil
	}

	if refused := s.earnedItsWayOut(ctx, domain.AgentID(req.AgentId), stage); refused != nil {
		return refused, nil
	}

	if err := s.promotions.SetStage(ctx, domain.AgentID(req.AgentId), stage, callerOf(ctx)); err != nil {
		return nil, fmt.Errorf("set stage of %s: %w", req.AgentId, err)
	}
	return openapi.SetAgentStage204Response{}, nil
}

/*
earnedItsWayOut refuses to let a draft out that has never been simulated.

FU-10: an agent cannot leave Draft without a simulation somebody ran. It is the
only check that exists before an agent touches real work, and a promotion that
skipped it would make the whole authoring path optional.

What it checks is that a simulation exists, not that anybody read it. Reviewing
is the half this platform cannot observe — it can put the report in front of a
person and record that somebody asked for it, and it cannot know they thought
about it.

Demotion is never refused, whatever the state. The platform demotes on its own
when people keep overruling an agent, and a person must never have less power
than the sweep.
*/
func (s *Server) earnedItsWayOut(
	ctx context.Context, agent domain.AgentID, to domain.Stage,
) openapi.SetAgentStageResponseObject {
	if to == domain.StageDraft || s.store == nil {
		return nil
	}
	was, err := s.promotions.StageOf(ctx, agent)
	if err != nil || was != domain.StageDraft {
		return nil
	}

	simulated, err := s.store.HasSimulation(ctx, agent)
	if err != nil || simulated {
		return nil
	}
	return openapi.SetAgentStage400ApplicationProblemPlusJSONResponse(problem(
		http.StatusBadRequest, "Not simulated yet",
		"An agent leaves Draft by being run against occurrences that already happened. Simulate it first.",
	))
}
