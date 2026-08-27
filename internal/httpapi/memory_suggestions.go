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

// The queue of proposals an agent made, and the two ways out of it.
//
// Accepting merges through the store's one merge; dismissing records a refusal.
// Neither is a write a person composed, which is why the body carries a reason
// and little else.

func (s *Server) ListMemorySuggestions(
	ctx context.Context, req openapi.ListMemorySuggestionsRequestObject,
) (openapi.ListMemorySuggestionsResponseObject, error) {
	scopes, refused := suggestionReadableScopes(ctx, req.Params)
	if refused != nil {
		return openapi.ListMemorySuggestions403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	if s.memory == nil {
		return openapi.ListMemorySuggestions200JSONResponse{Items: []openapi.MemorySuggestion{}}, nil
	}
	items, err := s.memory.ListSuggestions(ctx, suggestionFilter(scopes, req.Params))
	if err != nil {
		return nil, fmt.Errorf("list memory suggestions: %w", err)
	}
	return openapi.ListMemorySuggestions200JSONResponse{Items: memorySuggestions(items)}, nil
}

func (s *Server) AcceptMemorySuggestion(
	ctx context.Context, req openapi.AcceptMemorySuggestionRequestObject,
) (openapi.AcceptMemorySuggestionResponseObject, error) {
	if s.memory == nil || req.Body == nil {
		return badMemorySuggestionAccept("memory suggestion review body is required"), nil
	}
	scope, err := inputScope(req.Body.Company, req.Body.Area)
	if err != nil {
		return badMemorySuggestionAccept(err.Error()), nil
	}
	if err := auth.Require(ctx, domain.PermAgentPublish, scope); err != nil {
		return forbiddenMemorySuggestionAccept(domain.PermAgentPublish, scope), nil
	}
	// The same classifier and the same override as creation, on the words being
	// agreed to. A memory reached through the queue is quoted back into runs
	// exactly like one somebody typed, so a policy that applied to only one of
	// the two doors would be a policy with a door around it.
	claim := valueOr(req.Body.Claim)
	override := req.Body.OverrideSecretWarning != nil && *req.Body.OverrideSecretWarning
	if refused := secretRefusal(override, claim, req.Body.Reason); refused != nil {
		return openapi.AcceptMemorySuggestion400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(*refused),
		}, nil
	}
	assertion, err := s.memory.AcceptSuggestion(ctx, memstore.AcceptInput{
		ID: req.SuggestionId, Scope: scope, By: callerOf(ctx),
		Reason: req.Body.Reason, Claim: claim, Now: clockOr(s.clock).Now(),
	})
	if errors.Is(err, memstore.ErrNotFound) {
		return openapi.AcceptMemorySuggestion404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.SuggestionId),
		}, nil
	}
	switch memoryRefusal(err) {
	case http.StatusConflict:
		return openapi.AcceptMemorySuggestion409ApplicationProblemPlusJSONResponse(
			conflicted(err.Error())), nil
	case http.StatusBadRequest:
		return badMemorySuggestionAccept(err.Error()), nil
	}
	if err != nil {
		return nil, fmt.Errorf("accept memory suggestion %s: %w", req.SuggestionId, err)
	}
	return openapi.AcceptMemorySuggestion200JSONResponse(memoryAssertion(assertion)), nil
}

func (s *Server) DismissMemorySuggestion(
	ctx context.Context, req openapi.DismissMemorySuggestionRequestObject,
) (openapi.DismissMemorySuggestionResponseObject, error) {
	if s.memory == nil || req.Body == nil {
		return badMemorySuggestionDismiss("memory suggestion review body is required"), nil
	}
	scope, err := inputScope(req.Body.Company, req.Body.Area)
	if err != nil {
		return badMemorySuggestionDismiss(err.Error()), nil
	}
	if err := auth.Require(ctx, domain.PermAgentPublish, scope); err != nil {
		return forbiddenMemorySuggestionDismiss(domain.PermAgentPublish, scope), nil
	}
	err = s.memory.DismissSuggestion(ctx, req.SuggestionId, scope,
		callerOf(ctx), req.Body.Reason, clockOr(s.clock).Now())
	if errors.Is(err, memstore.ErrNotFound) {
		return openapi.DismissMemorySuggestion404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.SuggestionId),
		}, nil
	}
	if memoryRefusal(err) == http.StatusBadRequest {
		return badMemorySuggestionDismiss(err.Error()), nil
	}
	if err != nil {
		return nil, fmt.Errorf("dismiss memory suggestion %s: %w", req.SuggestionId, err)
	}
	return openapi.DismissMemorySuggestion204Response{}, nil
}

func badMemorySuggestionAccept(detail string) openapi.AcceptMemorySuggestion400ApplicationProblemPlusJSONResponse {
	return openapi.AcceptMemorySuggestion400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			invalid(detail)),
	}
}

func forbiddenMemorySuggestionAccept(
	perm domain.Permission, scope domain.Scope,
) openapi.AcceptMemorySuggestion403ApplicationProblemPlusJSONResponse {
	return openapi.AcceptMemorySuggestion403ApplicationProblemPlusJSONResponse{
		ForbiddenApplicationProblemPlusJSONResponse: forbidden(perm, scope),
	}
}

func badMemorySuggestionDismiss(detail string) openapi.DismissMemorySuggestion400ApplicationProblemPlusJSONResponse {
	return openapi.DismissMemorySuggestion400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			invalid(detail)),
	}
}

func forbiddenMemorySuggestionDismiss(
	perm domain.Permission, scope domain.Scope,
) openapi.DismissMemorySuggestion403ApplicationProblemPlusJSONResponse {
	return openapi.DismissMemorySuggestion403ApplicationProblemPlusJSONResponse{
		ForbiddenApplicationProblemPlusJSONResponse: forbidden(perm, scope),
	}
}
