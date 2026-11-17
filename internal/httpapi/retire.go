package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
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
	Retire(ctx context.Context, agent domain.AgentID, by domain.UserID, guard spec.Guard) error
	Restore(ctx context.Context, agent domain.AgentID, by domain.UserID, guard spec.Guard) error
	Retired(ctx context.Context) (map[domain.AgentID]bool, error)
}

// WithRetirements wires taking an agent out of circulation.
func (s *Server) WithRetirements(retirements Retirements) *Server {
	s.retirements = retirements
	return s
}

// SetAgentRetired takes an agent out of circulation, or brings it back.
//
// Who may is judged by the store under the agent's lock, on the scope it lives
// in then (NT-017 phase 1): the authority to publish. Retiring is an authoring
// decision about whether this agent is still part of the estate, not an
// operational one about whether it should be acting this afternoon — that is
// pausing, and it asks for something else.
func (s *Server) SetAgentRetired(
	ctx context.Context, req openapi.SetAgentRetiredRequestObject,
) (openapi.SetAgentRetiredResponseObject, error) {
	absent := openapi.SetAgentRetired404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}
	guard, bad := agentGuard(ctx, req.Params.IfMatch, nil)
	if bad != "" {
		return openapi.SetAgentRetired400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(bad)),
		}, nil
	}
	current, ok := s.currentAgent(ctx, domain.AgentID(req.AgentId))
	if !ok || s.retirements == nil || req.Body == nil {
		return absent, nil
	}

	act := s.retirements.Restore
	if req.Body.Retired {
		act = s.retirements.Retire
	}
	err := act(ctx, current.ID, callerOf(ctx), guard)
	if agentAbsent(err) {
		return absent, nil
	}
	if stale, ok := staleFrom(err); ok {
		return openapi.SetAgentRetired412ApplicationProblemPlusJSONResponse{
			PreconditionFailedApplicationProblemPlusJSONResponse: stale,
		}, nil
	}
	if err != nil {
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
