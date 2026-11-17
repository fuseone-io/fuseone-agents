package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"

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
	// The draft as it is now, for what accepting a block proposal would leave.
	opened, err := s.drafts.Get(ctx, domain.AgentID(req.AgentId), draftCaller(ctx))
	if errors.Is(err, drafts.ErrNotFound) {
		return absent, nil
	}
	if err != nil {
		return nil, err
	}
	effects := s.proposedToolEffects(ctx, opened, found)
	items := make([]openapi.DraftProposal, 0, len(found))
	for _, p := range found {
		view := proposalView(p)
		if after, ok := instructionsAfter(opened, p); ok {
			view.InstructionsAfter = ptr(after)
		}
		if effect, ok := effects(p); ok {
			view.ToolEffect = ptr(openapi.DraftProposalToolEffect(effect))
		}
		items = append(items, view)
	}
	return openapi.ListDraftProposals200JSONResponse{Items: items}, nil
}

/*
instructionsAfter is what accepting a block proposal now would leave of the
instructions, rendered by the codec accepting uses, so the card shows the
effect and not a guess at it: the label a new or rewritten block takes in its
locale, its position, the block a move takes. Only for a proposal that could
be accepted now; any other would be refused, and shows nothing to accept.
*/
func instructionsAfter(opened drafts.Opened, p drafts.Proposal) (string, bool) {
	if p.Op == nil || !isBlockKind(p.Op.Kind()) || !p.Valid || p.Decision != "" || p.Read != opened.Revision {
		return "", false
	}
	next, err := draft.Preview(opened.Draft, p.Op)
	if err != nil {
		return "", false
	}
	return next.Definition().Instructions, true
}

/*
proposedToolEffects answers, for a waiting valid tool proposal, the effect the
catalogue rules for its tool in the draft's stored scope: the scope accepting
is judged in, never one a console has not saved. A tool nobody ruled on there
is unknown. The catalogue is read once, and only when a proposal needs it;
read or not, the list is answered, and without an effect a client offers no
acceptance.
*/
func (s *Server) proposedToolEffects(ctx context.Context, opened drafts.Opened, found []drafts.Proposal) func(drafts.Proposal) (string, bool) {
	none := func(drafts.Proposal) (string, bool) { return "", false }
	toolOf := func(p drafts.Proposal) (domain.ToolID, bool) {
		if !p.Valid || p.Decision != "" {
			return "", false
		}
		switch op := p.Op.(type) {
		case draft.AddTool:
			return op.Tool, true
		case draft.RemoveTool:
			return op.Tool, true
		}
		return "", false
	}
	if !slices.ContainsFunc(found, func(p drafts.Proposal) bool { _, ok := toolOf(p); return ok }) {
		return none
	}
	rulings, err := s.checker().Rulings(ctx, domain.Scope{Company: opened.Draft.Spec.Company, Area: opened.Draft.Spec.Area})
	if err != nil {
		return none
	}
	return func(p drafts.Proposal) (string, bool) {
		tool, ok := toolOf(p)
		if !ok {
			return "", false
		}
		effect, _ := rulings.Effect(tool)
		return effect.String(), true
	}
}

func isBlockKind(kind string) bool {
	switch kind {
	case "insert_block", "replace_block", "remove_block", "move_block":
		return true
	}
	return false
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
	if p.Reason != "" {
		out.Reason = ptr(p.Reason)
	}
	return out
}
