package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Taking an agent out of circulation.

There is no delete here and there will not be one: a run is pinned to a
version and that version is the only correct explanation of what the run did.
What was missing is the honest alternative — an agent nobody uses any more
that stops appearing on every screen without taking its history with it.
*/

// Retirements takes agents out of circulation, declared here by the consumer.
type Retirements interface {
	Retire(ctx context.Context, agent domain.AgentID, by domain.UserID) error
	Restore(ctx context.Context, agent domain.AgentID, by domain.UserID) error
	Retired(ctx context.Context) (map[domain.AgentID]bool, error)
}

// WithRetirements wires taking an agent out of circulation.
func (s *Server) WithRetirements(retirements Retirements) *Server {
	s.retirements = retirements
	return s
}

// SetAgentRetired takes an agent out of circulation, or brings it back.
func (s *Server) SetAgentRetired(
	ctx context.Context, req openapi.SetAgentRetiredRequestObject,
) (openapi.SetAgentRetiredResponseObject, error) {
	current, ok := s.currentAgent(ctx, domain.AgentID(req.AgentId))
	if !ok || s.retirements == nil || req.Body == nil {
		return openapi.SetAgentRetired404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
		}, nil
	}

	// The authority to publish. Retiring is an authoring decision
	// about whether this agent is still part of the estate, not an operational
	// one about whether it should be acting this afternoon — that is pausing,
	// and it asks for something else.
	if err := auth.Require(ctx, domain.PermAgentPublish, current.Scope); err != nil {
		return openapi.SetAgentRetired403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, current.Scope),
		}, nil
	}

	act := s.retirements.Restore
	if req.Body.Retired {
		act = s.retirements.Retire
	}
	if err := act(ctx, current.ID, callerOf(ctx)); err != nil {
		return nil, fmt.Errorf("set %s retired=%v: %w", req.AgentId, req.Body.Retired, err)
	}
	return openapi.SetAgentRetired204Response{}, nil
}

// isRetired answers whether one agent is out of circulation.
func (s *Server) isRetired(ctx context.Context, agent domain.AgentID) (bool, error) {
	if s.retirements == nil {
		return false, nil
	}
	retired, err := s.retirements.Retired(ctx)
	if err != nil {
		return false, fmt.Errorf("retired agents: %w", err)
	}
	return retired[agent], nil
}
