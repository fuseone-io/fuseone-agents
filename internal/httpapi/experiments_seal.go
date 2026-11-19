package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
	"github.com/fuseone/agents/internal/spec"
)

// experimentTarget is what an experiment runs on: a published version, or a candidate
// (NT-018 phase 2) whose sealed definition says which provider it asks.
type experimentTarget struct {
	summary domain.AgentSummary
	// unreadable is a candidate whose seal is erased or no longer matches its claim. A
	// first attempt on it is refused; a retry with runs answers from them.
	unreadable bool
}

/*
experimentTarget resolves what this request runs on, or the answer that refuses it.

A request naming a seal, or a retry of an experiment recorded on one, runs the candidate:
authorised on the sealed scope, answered 404 outside it as for a seal that does not exist,
and with no published version needed. Anything else runs a published version, as before.
*/
func (s *Server) experimentTarget(
	ctx context.Context, agent domain.AgentID, id string, ask experimentAsk,
) (experimentTarget, openapi.StartExperimentResponseObject, error) {
	seal, err := s.experimentSeal(ctx, id, ask)
	if err != nil {
		return experimentTarget{}, nil, err
	}
	if seal == "" {
		return s.publishedTarget(ctx, agent)
	}
	absent := experimentAbsent(string(agent))
	if s.candidates == nil {
		return experimentTarget{}, absent, nil
	}
	c, found, err := s.candidates.Candidate(ctx, agent, seal)
	if err != nil || !found || auth.Require(ctx, domain.PermAgentPublish, c.Scope) != nil {
		return experimentTarget{}, absent, err
	}
	target := experimentTarget{summary: domain.AgentSummary{ID: agent, VersionID: c.Version, Scope: c.Scope}}
	sealed, err := s.candidates.Sealed(ctx, agent, seal)
	switch {
	case errors.Is(err, domain.ErrCandidateUnreadable):
		target.unreadable = true
	case errors.Is(err, spec.ErrCandidateNotFound):
		return experimentTarget{}, absent, nil
	case err != nil:
		return experimentTarget{}, nil, err
	default:
		target.summary.Provider = sealed.Provider
	}
	return target, nil, nil
}

/*
experimentSeal is the seal this request runs on: the one its experiment was recorded on,
when it was recorded, and otherwise the one it names.

The recorded intention comes first. A retry is answered by what its key already names, and
only then compared with what it asks (settle): a retry naming another seal, or one that
does not exist, is the conflict of an intention that diverges, never a 404 for the seal it
named instead.
*/
func (s *Server) experimentSeal(ctx context.Context, id string, ask experimentAsk) (string, error) {
	recorded, found, err := simulate.Recorded(ctx, s.store, id)
	if err != nil {
		return "", err
	}
	if found {
		return recorded.Seal, nil
	}
	if ask.seal != nil {
		return *ask.seal, nil
	}
	return "", nil
}

// publishedTarget is the version published now, refused as the route always has.
func (s *Server) publishedTarget(
	ctx context.Context, agent domain.AgentID,
) (experimentTarget, openapi.StartExperimentResponseObject, error) {
	published, ok, err := s.publishedAgent(ctx, string(agent))
	if err != nil || !ok {
		return experimentTarget{}, experimentAbsent(string(agent)), err
	}
	if err := auth.Require(ctx, domain.PermAgentPublish, published.Scope); err != nil {
		return experimentTarget{}, openapi.StartExperiment403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, published.Scope),
		}, nil
	}
	return experimentTarget{summary: published}, nil, nil
}

func experimentAbsent(id string) openapi.StartExperimentResponseObject {
	return openapi.StartExperiment404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(id)}
}

func candidateUnreadable(id string) openapi.StartExperimentResponseObject {
	return openapi.StartExperiment409ApplicationProblemPlusJSONResponse(refusal(http.StatusConflict,
		CodeCandidateUnreadable, "Candidate unreadable", fmt.Sprintf("experiment %s: seal the draft again", id)))
}
