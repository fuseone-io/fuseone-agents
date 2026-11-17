package httpapi

import (
	"errors"
	"net/http"

	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

// publicationConflict is a publication refused by something the publisher can
// act on: the stage it asked not to publish to, or a version id that names
// other bytes or bytes nobody kept (NT-018).
func publicationConflict(err error) (openapi.PublishAgent409ApplicationProblemPlusJSONResponse, bool) {
	for _, one := range []struct {
		err   error
		code  Code
		title string
	}{
		{spec.ErrStageRefused, CodeStageRefused, "Stage refused"},
		{spec.ErrVersionCollision, CodeVersionCollision, "Version id already taken"},
		{spec.ErrDigestUnknown, CodeVersionDigestUnknown, "Version published without a digest"},
	} {
		if errors.Is(err, one.err) {
			return openapi.PublishAgent409ApplicationProblemPlusJSONResponse(
				refusal(http.StatusConflict, one.code, one.title, err.Error())), true
		}
	}
	return openapi.PublishAgent409ApplicationProblemPlusJSONResponse{}, false
}

// publicationRefused is a publication the store refused for a reason the
// caller can act on: the guard's 404 or 412, or a 409.
func publicationRefused(agent string, err error) (openapi.PublishAgentResponseObject, bool) {
	if agentAbsent(err) {
		return openapi.PublishAgent404ApplicationProblemPlusJSONResponse(
			refusal(http.StatusNotFound, CodeNotFound, "Not found", agent)), true
	}
	if stale, ok := staleFrom(err); ok {
		return openapi.PublishAgent412ApplicationProblemPlusJSONResponse{
			PreconditionFailedApplicationProblemPlusJSONResponse: stale,
		}, true
	}
	return publicationConflict(err)
}
