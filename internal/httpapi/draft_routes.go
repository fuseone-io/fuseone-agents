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
The workbench's draft, over HTTP (NT-018 1b, part 3).

The store decides: who may reach a draft, on its scope and where its agent
lives now, and every precondition, under its locks. The route reads the
headers by syntax, refuses what the contract refuses before anything stored
is touched — a block operation, a write with no precondition — and turns
each refusal into its answer.
*/

// Drafts is the draft store, declared here by the consumer.
type Drafts interface {
	Get(ctx context.Context, agent domain.AgentID, c drafts.Caller) (drafts.Opened, error)
	Create(ctx context.Context, agent domain.AgentID, identity *drafts.Identity, c drafts.Caller) (drafts.Opened, error)
	Apply(ctx context.Context, a drafts.Application) (drafts.Opened, error)
	Discard(ctx context.Context, agent domain.AgentID, expect domain.IfMatch, c drafts.Caller) error
	Proposals(ctx context.Context, agent domain.AgentID, c drafts.Caller) ([]drafts.Proposal, error)
	Accept(ctx context.Context, a drafts.Acceptance) (drafts.Opened, error)
	Reject(ctx context.Context, agent domain.AgentID, id string, c drafts.Caller) error
	// What a turn needs from the store (conversation.Store), and the thread's
	// own reading and erasure.
	Turns(ctx context.Context, agent domain.AgentID, c drafts.Caller) ([]drafts.Turn, error)
	ReplayTurn(ctx context.Context, r drafts.Replaying) (drafts.Started, bool, error)
	BeginTurn(ctx context.Context, b drafts.Beginning) (drafts.Started, error)
	SettleTurn(ctx context.Context, s drafts.Settlement) (drafts.Settled, error)
	EraseThread(ctx context.Context, agent domain.AgentID, c drafts.Caller) error
}

// WithDrafts wires the workbench's drafts.
func (s *Server) WithDrafts(d Drafts) *Server {
	s.drafts = d
	return s
}

func draftCaller(ctx context.Context) drafts.Caller {
	return drafts.Caller{By: callerOf(ctx), Allow: mayPublishAt(ctx)}
}

// GetAgentDraft reads the agent's draft.
func (s *Server) GetAgentDraft(
	ctx context.Context, req openapi.GetAgentDraftRequestObject,
) (openapi.GetAgentDraftResponseObject, error) {
	absent := openapi.GetAgentDraft404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}
	if s.drafts == nil {
		return absent, nil
	}
	opened, err := s.drafts.Get(ctx, domain.AgentID(req.AgentId), draftCaller(ctx))
	if errors.Is(err, drafts.ErrNotFound) {
		return absent, nil
	}
	if err != nil {
		return nil, err
	}
	return openapi.GetAgentDraft200JSONResponse{
		Body: draftView(opened), Headers: openapi.GetAgentDraft200ResponseHeaders{ETag: string(opened.Revision)},
	}, nil
}

// CreateAgentDraft opens the agent's draft.
func (s *Server) CreateAgentDraft(
	ctx context.Context, req openapi.CreateAgentDraftRequestObject,
) (openapi.CreateAgentDraftResponseObject, error) {
	absent, err := domain.ParseIfNoneMatch(&req.Params.IfNoneMatch)
	if err != nil || !absent {
		return openapi.CreateAgentDraft400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("a draft is opened with If-None-Match: *")),
		}, nil
	}
	if s.drafts == nil {
		return openapi.CreateAgentDraft404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
		}, nil
	}
	var identity *drafts.Identity
	if req.Body != nil {
		identity = &drafts.Identity{Name: req.Body.Name, Scope: domain.Scope{
			Company: domain.CompanyID(req.Body.Company), Area: domain.AreaID(req.Body.Area),
		}}
	}
	opened, err := s.drafts.Create(ctx, domain.AgentID(req.AgentId), identity, draftCaller(ctx))
	if err != nil {
		return createRefused(req.AgentId, err)
	}
	return openapi.CreateAgentDraft201JSONResponse{
		Body: draftView(opened), Headers: openapi.CreateAgentDraft201ResponseHeaders{ETag: string(opened.Revision)},
	}, nil
}

func createRefused(agent string, err error) (openapi.CreateAgentDraftResponseObject, error) {
	var denied *drafts.ForbiddenError
	if stale, ok := staleFrom(err); ok {
		return openapi.CreateAgentDraft412ApplicationProblemPlusJSONResponse{PreconditionFailedApplicationProblemPlusJSONResponse: stale}, nil
	}
	switch {
	case errors.Is(err, drafts.ErrNotFound):
		return openapi.CreateAgentDraft404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(agent)}, nil
	case errors.As(err, &denied):
		return openapi.CreateAgentDraft403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, denied.Scope),
		}, nil
	case errors.Is(err, drafts.ErrAgentExists):
		return openapi.CreateAgentDraft409ApplicationProblemPlusJSONResponse(
			refusal(http.StatusConflict, CodeConflict, "Agent already published", err.Error())), nil
	case invalidDraftChange(err):
		return openapi.CreateAgentDraft400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	return nil, err
}

// invalidDraftChange is a refusal the sender can fix by sending another
// operation: what it said is wrong, unfinished or too large.
func invalidDraftChange(err error) bool {
	for _, refused := range []error{
		draft.ErrRefused, draft.ErrInvalidOperation, draft.ErrTooLarge,
		draft.ErrConversationOnly, draft.ErrHumanOnly, drafts.ErrPreconditionRequired,
	} {
		if errors.Is(err, refused) {
			return true
		}
	}
	return false
}

// DiscardAgentDraft removes the agent's draft.
func (s *Server) DiscardAgentDraft(
	ctx context.Context, req openapi.DiscardAgentDraftRequestObject,
) (openapi.DiscardAgentDraftResponseObject, error) {
	expect, problem := requiredIfMatch(req.Params.IfMatch)
	if problem != "" {
		return openapi.DiscardAgentDraft400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(problem)),
		}, nil
	}
	absent := openapi.DiscardAgentDraft404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId)}
	if s.drafts == nil {
		return absent, nil
	}
	err := s.drafts.Discard(ctx, domain.AgentID(req.AgentId), expect, draftCaller(ctx))
	if stale, ok := staleFrom(err); ok {
		return openapi.DiscardAgentDraft412ApplicationProblemPlusJSONResponse{PreconditionFailedApplicationProblemPlusJSONResponse: stale}, nil
	}
	if errors.Is(err, drafts.ErrNotFound) {
		return absent, nil
	}
	if err != nil {
		return nil, err
	}
	return openapi.DiscardAgentDraft204Response{}, nil
}

// requiredIfMatch reads an If-Match every change to a draft must carry. The
// contract makes the header required, so the router refuses its absence; an
// empty one is refused here as the same mistake.
func requiredIfMatch(header string) (domain.IfMatch, string) {
	if header == "" {
		return domain.IfMatch{}, "a change to a draft names the ETag it read, in If-Match"
	}
	expect, err := domain.ParseIfMatch(&header)
	if err != nil {
		return domain.IfMatch{}, err.Error()
	}
	return expect, ""
}
