package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// RemovePerson deactivates an account made here and takes it off the list.
// The store decides what may go; this names who asked and when.
func (s *Server) RemovePerson(
	ctx context.Context, req openapi.RemovePersonRequestObject,
) (openapi.RemovePersonResponseObject, error) {
	if err := auth.Require(ctx, domain.PermIdentityWrite, identityScope); err != nil {
		return openapi.RemovePerson403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermIdentityWrite, identityScope),
		}, nil
	}
	if s.people == nil {
		return nil, errNoAdministration
	}

	err := s.people.RemoveLocal(ctx, auth.Removal{
		PrincipalID: req.PrincipalId, By: string(callerOf(ctx)), At: clockOr(s.clock).Now(),
	})
	if err == nil {
		return openapi.RemovePerson204Response{}, nil
	}
	if errors.Is(err, auth.ErrNoPrincipal) {
		return openapi.RemovePerson404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.PrincipalId),
		}, nil
	}
	if code, refused := removalRefusal(err); refused {
		return openapi.RemovePerson409ApplicationProblemPlusJSONResponse(
			refusal(http.StatusConflict, code, "Conflict", req.PrincipalId)), nil
	}
	return nil, fmt.Errorf("remove %s: %w", req.PrincipalId, err)
}

// removalRefusal names the refusals both people doors answer with 409.
func removalRefusal(err error) (Code, bool) {
	switch {
	case errors.Is(err, auth.ErrNotLocal):
		return CodePersonNotLocal, true
	case errors.Is(err, auth.ErrRemovingYourself):
		return CodePersonIsYou, true
	case errors.Is(err, auth.ErrRemoved):
		return CodePersonRemoved, true
	case errors.Is(err, auth.ErrLastAdministrator):
		return CodeLastAdministrator, true
	}
	return "", false
}
