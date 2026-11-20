package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
)

/*
Candidates, for display (NT-018 phase 2, release 2a).

Every seal is authorised on its own sealed scope with agent:publish, never on where the
agent lives now: an author in the agent's new area gains nothing sealed in the old one.
The reader decides which seals the caller may see; the routes only answer.
*/

// Candidates is the candidate reader, declared here by the consumer.
type Candidates interface {
	List(ctx context.Context, agent domain.AgentID, visible []domain.Scope) ([]spec.CandidateSeal, error)
	Read(ctx context.Context, agent domain.AgentID, version domain.VersionID, visible []domain.Scope) (spec.Candidate, error)
	Seal(ctx context.Context, agent domain.AgentID, seal string, visible []domain.Scope) (spec.CandidateSeal, error)
	// Candidate and Sealed resolve one seal for an experiment: what the opener runs it
	// as, and its definition checked as the worker checks it.
	Candidate(ctx context.Context, agent domain.AgentID, seal string) (trigger.Candidate, bool, error)
	Sealed(ctx context.Context, agent domain.AgentID, seal string) (spec.Spec, error)
}

// WithCandidates wires the candidate reader.
func (s *Server) WithCandidates(c Candidates) *Server {
	s.candidates = c
	return s
}

// ListAgentCandidates is the caller's candidate seals of an agent.
func (s *Server) ListAgentCandidates(
	ctx context.Context, req openapi.ListAgentCandidatesRequestObject,
) (openapi.ListAgentCandidatesResponseObject, error) {
	absent := openapi.ListAgentCandidates404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}
	if s.candidates == nil {
		return absent, nil
	}
	seals, err := s.candidates.List(ctx, domain.AgentID(req.AgentId), publishScopes(ctx))
	if err != nil {
		return nil, err
	}
	if len(seals) == 0 {
		return absent, nil
	}
	items := make([]openapi.CandidateSeal, 0, len(seals))
	for _, seal := range seals {
		items = append(items, candidateSealView(seal))
	}
	return openapi.ListAgentCandidates200JSONResponse{Items: items}, nil
}

// GetAgentSeal is one candidate seal, by its id: what an experiment on it will run.
func (s *Server) GetAgentSeal(
	ctx context.Context, req openapi.GetAgentSealRequestObject,
) (openapi.GetAgentSealResponseObject, error) {
	absent := openapi.GetAgentSeal404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(req.SealId)}
	if s.candidates == nil {
		return absent, nil
	}
	seal, err := s.candidates.Seal(ctx, domain.AgentID(req.AgentId), req.SealId, publishScopes(ctx))
	switch {
	case errors.Is(err, spec.ErrCandidateNotFound):
		return absent, nil
	case err != nil:
		return nil, err
	}
	return openapi.GetAgentSeal200JSONResponse(candidateSealView(seal)), nil
}

// GetAgentCandidate is a candidate's sealed text.
func (s *Server) GetAgentCandidate(
	ctx context.Context, req openapi.GetAgentCandidateRequestObject,
) (openapi.GetAgentCandidateResponseObject, error) {
	absent := openapi.GetAgentCandidate404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.Version),
	}
	if s.candidates == nil {
		return absent, nil
	}
	got, err := s.candidates.Read(ctx, domain.AgentID(req.AgentId), domain.VersionID(req.Version), publishScopes(ctx))
	switch {
	case errors.Is(err, spec.ErrCandidateNotFound):
		return absent, nil
	case errors.Is(err, domain.ErrCandidateUnreadable):
		return openapi.GetAgentCandidate409ApplicationProblemPlusJSONResponse(refusal(
			http.StatusConflict, CodeCandidateUnreadable, "Candidate unreadable", req.Version)), nil
	case err != nil:
		return nil, err
	}
	view := candidateSealView(got.CandidateSeal)
	return openapi.GetAgentCandidate200JSONResponse{
		Version: view.Version, SealId: view.SealId, Company: view.Company, Area: view.Area,
		SealedBy: view.SealedBy, SealedAt: view.SealedAt, Definition: got.Definition,
	}, nil
}

// publishScopes is where the caller may publish, for the reader to filter seals by in its
// query rather than reading every seal and discarding.
func publishScopes(ctx context.Context) []domain.Scope {
	return auth.VisibleScopes(ctx, domain.PermAgentPublish)
}

func candidateSealView(seal spec.CandidateSeal) openapi.CandidateSeal {
	return openapi.CandidateSeal{
		Version: string(seal.Version), SealId: seal.SealID,
		Company: string(seal.Scope.Company), Area: string(seal.Scope.Area),
		SealedBy: string(seal.SealedBy), SealedAt: seal.SealedAt.UTC(),
	}
}
