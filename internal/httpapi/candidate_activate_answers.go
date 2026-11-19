package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
)

// activationConflicts are the store's refusals answered 409, each with its own type.
var activationConflicts = []struct {
	err   error
	code  Code
	title string
}{
	{spec.ErrCandidateScopeMoved, CodeCandidateScopeMoved, "The agent moved out of the version's scope"},
	{spec.ErrStageRefused, CodeStageRefused, "Stage refused"},
	{domain.ErrCandidateUnreadable, CodeCandidateUnreadable, "Candidate unreadable"},
	{spec.ErrVersionCollision, CodeVersionCollision, "Version id already taken"},
	{spec.ErrDigestUnknown, CodeVersionDigestUnknown, "Version published without a digest"},
	{trigger.ErrPathTaken, CodeWebhookPathTaken, "Webhook path taken"},
}

// activationRefused is an activation the store refused for a reason the caller can act
// on, answered as a publication answers the same cause.
func activationRefused(agent string, err error) (activationReply, bool) {
	switch {
	case err == nil:
		return nil, false
	case errors.Is(err, errNeedsNever):
		return activationInvalid(CodePublishNeedsNever, err.Error()), true
	case agentAbsent(err), errors.Is(err, spec.ErrCandidateNotFound):
		return activationAbsent(agent), true
	}
	if stale, ok := staleFrom(err); ok {
		return openapi.ActivateAgentVersion412ApplicationProblemPlusJSONResponse{
			PreconditionFailedApplicationProblemPlusJSONResponse: stale,
		}, true
	}
	for _, one := range activationConflicts {
		if errors.Is(err, one.err) {
			return openapi.ActivateAgentVersion409ApplicationProblemPlusJSONResponse(
				refusal(http.StatusConflict, one.code, one.title, err.Error())), true
		}
	}
	return nil, false
}

func activationUnreadable(agent domain.AgentID, version domain.VersionID) activationReply {
	return openapi.ActivateAgentVersion409ApplicationProblemPlusJSONResponse(refusal(
		http.StatusConflict, CodeCandidateUnreadable, "Candidate unreadable",
		fmt.Sprintf("no seal of %s@%s is readable: seal the draft again", agent, version)))
}

func activationRequired() activationReply {
	return openapi.ActivateAgentVersion428ApplicationProblemPlusJSONResponse(refusal(
		http.StatusPreconditionRequired, CodePreconditionRequired, "Precondition required",
		"send If-Match with the agent's ETag, or If-None-Match: * for a first version"))
}

func activationAbsent(agent string) activationReply {
	return openapi.ActivateAgentVersion404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(agent),
	}
}

func activationInvalid(code Code, detail string) activationReply {
	return openapi.ActivateAgentVersion400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			refusal(http.StatusBadRequest, code, "Not activated", detail)),
	}
}
