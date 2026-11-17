package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
The conversation's proposals, over HTTP (NT-018 phase 3, contract 3).

Proposals are made by the model in a person's turn, never through this API;
here a person reads their own thread and decides. Accepting carries nothing
but the id: the store loads what it kept and applies it.
*/

// ListDraftProposals is the caller's own thread over the draft.
func (s *Server) ListDraftProposals(
	ctx context.Context, req openapi.ListDraftProposalsRequestObject,
) (openapi.ListDraftProposalsResponseObject, error) {
	absent := openapi.ListDraftProposals404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId)}
	if s.drafts == nil {
		return absent, nil
	}
	found, err := s.drafts.Proposals(ctx, domain.AgentID(req.AgentId), draftCaller(ctx))
	if errors.Is(err, drafts.ErrNotFound) {
		return absent, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]openapi.DraftProposal, 0, len(found))
	for _, p := range found {
		items = append(items, proposalView(p))
	}
	return openapi.ListDraftProposals200JSONResponse{Items: items}, nil
}

// AcceptDraftProposal applies one proposal, by its id alone.
func (s *Server) AcceptDraftProposal(
	ctx context.Context, req openapi.AcceptDraftProposalRequestObject,
) (openapi.AcceptDraftProposalResponseObject, error) {
	if s.drafts == nil {
		return openapi.AcceptDraftProposal404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(req.ProposalId)}, nil
	}
	opened, err := s.drafts.Accept(ctx, drafts.Acceptance{
		Agent: domain.AgentID(req.AgentId), Proposal: req.ProposalId, Caller: draftCaller(ctx),
		Validator: draft.Validator{Checker: s.checker()}, Now: clockOr(s.clock).Now(),
	})
	if err == nil {
		return openapi.AcceptDraftProposal200JSONResponse{
			Body: draftView(opened), Headers: openapi.AcceptDraftProposal200ResponseHeaders{ETag: string(opened.Revision)},
		}, nil
	}
	return acceptRefused(req.ProposalId, err)
}

func acceptRefused(id string, err error) (openapi.AcceptDraftProposalResponseObject, error) {
	var denied *drafts.ForbiddenError
	if stale, ok := staleFrom(err); ok {
		return openapi.AcceptDraftProposal412ApplicationProblemPlusJSONResponse{PreconditionFailedApplicationProblemPlusJSONResponse: stale}, nil
	}
	switch {
	case errors.Is(err, drafts.ErrNotFound):
		return openapi.AcceptDraftProposal404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(id)}, nil
	case errors.Is(err, drafts.ErrDecided):
		return openapi.AcceptDraftProposal409ApplicationProblemPlusJSONResponse(
			refusal(http.StatusConflict, CodeConflict, "Proposal decided already", err.Error())), nil
	case errors.As(err, &denied):
		return openapi.AcceptDraftProposal403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, denied.Scope),
		}, nil
	case errors.Is(err, drafts.ErrProposalInvalid) || invalidDraftChange(err):
		return openapi.AcceptDraftProposal400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	return nil, err
}

// RejectDraftProposal decides against one proposal.
func (s *Server) RejectDraftProposal(
	ctx context.Context, req openapi.RejectDraftProposalRequestObject,
) (openapi.RejectDraftProposalResponseObject, error) {
	absent := openapi.RejectDraftProposal404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(req.ProposalId)}
	if s.drafts == nil {
		return absent, nil
	}
	err := s.drafts.Reject(ctx, domain.AgentID(req.AgentId), req.ProposalId, draftCaller(ctx))
	switch {
	case err == nil:
		return openapi.RejectDraftProposal204Response{}, nil
	case errors.Is(err, drafts.ErrNotFound):
		return absent, nil
	case errors.Is(err, drafts.ErrDecided):
		return openapi.RejectDraftProposal409ApplicationProblemPlusJSONResponse(
			refusal(http.StatusConflict, CodeConflict, "Proposal decided already", err.Error())), nil
	}
	return nil, err
}

func proposalView(p drafts.Proposal) openapi.DraftProposal {
	out := openapi.DraftProposal{
		ProposalId: p.ID, Kind: openapi.DraftProposalKind(p.Kind), Valid: p.Valid, ReadRevision: string(p.Read), CreatedAt: p.At,
	}
	if p.Op != nil {
		out.Kind = openapi.DraftProposalKind(p.Op.Kind())
		op := operationView(p.Op)
		out.Operation = &op
	}
	if p.Refusal != "" {
		out.Refusal = ptr(p.Refusal)
	}
	if p.Decision != "" {
		out.Decision = ptr(openapi.DraftProposalDecision(p.Decision))
	}
	if p.Turn != "" {
		out.TurnId = ptr(p.Turn)
	}
	return out
}
