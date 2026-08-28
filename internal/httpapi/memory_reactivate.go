package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	memstore "github.com/fuseone/agents/internal/memory"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Bringing a memory back, kept apart from recording and correcting one.
//
// It is the only memory act with a dependency of its own: the ledger, which is
// what the citations are proved against before the row becomes readable again.

/*
WithMemoryEvidence wires what a memory is proved against.

Separate from WithMemory because it is not part of storing one: reactivation
reads the ledger and the content store to check that the citations still hold,
and a store that could do that itself would be a store that reaches into the
ledger.

An installation that wires memory without this cannot reactivate. That is a
refusal rather than a skipped proof — bringing a memory back without checking it
is the thing this exists to prevent.
*/
func (s *Server) WithMemoryEvidence(led memstore.EvidenceLedger, content memstore.EvidenceContent) *Server {
	s.memoryEvidence = memstore.NewResolver(led, content)
	return s
}

func (s *Server) ReactivateMemoryAssertion(
	ctx context.Context, req openapi.ReactivateMemoryAssertionRequestObject,
) (openapi.ReactivateMemoryAssertionResponseObject, error) {
	if s.memory == nil || req.Body == nil {
		return badMemoryReactivate("memory reactivate body is required"), nil
	}
	scope, err := inputScope(req.Body.Company, req.Body.Area)
	if err != nil {
		return badMemoryReactivate(err.Error()), nil
	}
	if err := auth.Require(ctx, domain.PermAgentPublish, scope); err != nil {
		return forbiddenMemoryReactivate(domain.PermAgentPublish, scope), nil
	}
	if s.memoryEvidence == nil {
		return nil, fmt.Errorf("httpapi: no evidence resolver to prove %s against", req.AssertionId)
	}
	assertion, err := s.memory.Reactivate(ctx, s.memoryEvidence, memstore.ReactivateInput{
		ID: req.AssertionId, Scope: scope, By: callerOf(ctx),
		Reason: req.Body.Reason, Now: clockOr(s.clock).Now(),
	})
	if errors.Is(err, memstore.ErrNotFound) {
		return openapi.ReactivateMemoryAssertion404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.AssertionId),
		}, nil
	}
	switch memoryRefusal(err) {
	case http.StatusConflict:
		return openapi.ReactivateMemoryAssertion409ApplicationProblemPlusJSONResponse(
			conflicted(err.Error())), nil
	case http.StatusBadRequest:
		return badMemoryReactivate(err.Error()), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reactivate memory assertion %s: %w", req.AssertionId, err)
	}
	return openapi.ReactivateMemoryAssertion200JSONResponse(memoryAssertion(assertion)), nil
}

func badMemoryReactivate(detail string) openapi.ReactivateMemoryAssertion400ApplicationProblemPlusJSONResponse {
	return openapi.ReactivateMemoryAssertion400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			invalid(detail)),
	}
}

func forbiddenMemoryReactivate(
	perm domain.Permission, scope domain.Scope,
) openapi.ReactivateMemoryAssertion403ApplicationProblemPlusJSONResponse {
	return openapi.ReactivateMemoryAssertion403ApplicationProblemPlusJSONResponse{
		ForbiddenApplicationProblemPlusJSONResponse: forbidden(perm, scope),
	}
}
