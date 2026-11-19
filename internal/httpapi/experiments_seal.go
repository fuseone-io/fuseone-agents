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
	// seal is the candidate's seal, empty for a published version. Its bytes are read
	// inside the experiment's fence (withSealedDefinition), never before it.
	seal string
	// unreadable is a candidate whose seal is erased or no longer matches its claim. A
	// first attempt on it is refused; a retry with runs answers from them.
	unreadable bool
}

// experimentCall is one request to start an experiment: its agent, its id, what it asks,
// and why its body is refused, when it is.
type experimentCall struct {
	agent   domain.AgentID
	id      string
	ask     experimentAsk
	refused error
}

/*
resolvedTarget is what the experiment runs on, decided inside its fence.

All of it: which seal (the recorded intention's first, then the request's), the seal's row,
the authorisation, the published version's refusals, and the seal's bytes. A call that
reached the route before any intention existed decides nothing on the way in, so it cannot
enter carrying a published target after another call recorded the experiment on a seal,
skip the seal's bytes, and open runs on a seal erased in between. A refusal is set in
out.answer, in the order the route always answered: 404 or 403, then the request's 400.
*/
func (s *Server) resolvedTarget(
	ctx context.Context, call experimentCall, out *experimentOpened,
) (experimentTarget, error) {
	target, answer, err := s.experimentTarget(ctx, call.agent, call.id, call.ask)
	if err != nil || answer != nil {
		out.answer = answer
		return experimentTarget{}, err
	}
	if call.refused != nil {
		out.answer = openapi.StartExperiment400ApplicationProblemPlusJSONResponse(invalid(call.refused.Error()))
		return experimentTarget{}, nil
	}
	return s.withSealedDefinition(ctx, target)
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
	return experimentTarget{
		summary: domain.AgentSummary{ID: agent, VersionID: c.Version, Scope: c.Scope}, seal: seal,
	}, nil, nil
}

/*
withSealedDefinition reads a candidate's bytes, inside the experiment's fence.

It runs inside the fence, after the seal's row was resolved there too (resolvedTarget). An
erasure of the seal takes the same fence, so a read here either comes after the erasure and
finds the tombstone, or comes first and opens its runs before the erasure reaches them.
Read before the fence, the bytes could be found readable, the erasure commit, and the runs
open on bytes that no longer exist.
*/
func (s *Server) withSealedDefinition(ctx context.Context, target experimentTarget) (experimentTarget, error) {
	if target.seal == "" {
		return target, nil
	}
	sealed, err := s.candidates.Sealed(ctx, target.summary.ID, target.seal)
	switch {
	case errors.Is(err, domain.ErrCandidateUnreadable), errors.Is(err, spec.ErrCandidateNotFound):
		target.unreadable = true
	case err != nil:
		return experimentTarget{}, err
	default:
		target.summary.Provider = sealed.Provider
	}
	return target, nil
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
