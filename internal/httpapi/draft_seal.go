package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

/*
Sealing the draft as a candidate (NT-018 phase 2, release 2b).

The route reads the draft the caller may reach, at the ETag they sent, and checks it as a
publication checks a definition, without the "never" block activation will ask for. The
store seals it under its locks, at that same revision, and decides what it writes.
*/

type sealResponse = openapi.SealAgentDraftResponseObject

// SealAgentDraft keeps the caller's draft as a candidate.
func (s *Server) SealAgentDraft(
	ctx context.Context, req openapi.SealAgentDraftRequestObject,
) (sealResponse, error) {
	agent := domain.AgentID(req.AgentId)
	if s.drafts == nil {
		return sealAbsent(req.AgentId), nil
	}
	expect, err := domain.ParseIfMatch(&req.Params.IfMatch)
	if err != nil {
		return sealInvalid(CodeInvalidInput, err.Error()), nil
	}
	opened, err := s.drafts.Get(ctx, agent, draftCaller(ctx))
	if errors.Is(err, drafts.ErrNotFound) {
		return sealAbsent(req.AgentId), nil
	}
	if err != nil {
		return nil, err
	}
	if !expect.Allows(true, opened.Revision) {
		return sealStale(opened.Revision), nil
	}
	if refusal, err := s.sealable(ctx, opened); refusal != nil || err != nil {
		return refusal, err
	}
	sealed, err := s.drafts.Seal(ctx, drafts.Sealing{Agent: agent, Expect: expect, Caller: draftCaller(ctx)})
	if refused, ok := sealRefused(req.AgentId, err); ok {
		return refused, nil
	}
	if err != nil {
		return nil, err
	}
	return sealAnswer(ctx, sealed), nil
}

// sealable refuses a draft a publication would refuse for its definition. The "never"
// block is not asked for, and the stage is not judged: those belong to activation.
func (s *Server) sealable(ctx context.Context, o drafts.Opened) (sealResponse, error) {
	_, definition, err := canonical(o.Draft.Definition())
	if err != nil {
		return sealInvalid(CodeInvalidInput, err.Error()), nil
	}
	detail, err := s.checker().Check(ctx, definition, clockOr(s.clock).Now())
	if err != nil || detail != "" {
		return sealInvalid(CodeInvalidInput, detail), err
	}
	return nil, nil
}

func sealRefused(agent string, err error) (sealResponse, bool) {
	if stale, ok := staleFrom(err); ok {
		return openapi.SealAgentDraft412ApplicationProblemPlusJSONResponse{PreconditionFailedApplicationProblemPlusJSONResponse: stale}, true
	}
	switch {
	case errors.Is(err, drafts.ErrNotFound):
		return sealAbsent(agent), true
	case errors.Is(err, spec.ErrVersionCollision):
		return sealConflict(CodeVersionCollision, "Version id already taken", err), true
	case errors.Is(err, spec.ErrDigestUnknown):
		return sealConflict(CodeVersionDigestUnknown, "Version published without a digest", err), true
	}
	return nil, false
}

func sealAnswer(ctx context.Context, sealed drafts.Sealed) sealResponse {
	body := openapi.DraftSeal{
		Version: string(sealed.Version), Published: sealed.Published,
		Company: string(sealed.Scope.Company), Area: string(sealed.Scope.Area),
		EventCycleWarning: eventCycleWarning(sealed.Judged, auth.VisibleScopes(ctx, domain.PermAgentRead)),
	}
	if sealed.Published {
		return openapi.SealAgentDraft200JSONResponse(body)
	}
	body.SealId = &sealed.SealID
	return openapi.SealAgentDraft201JSONResponse(body)
}

func sealAbsent(agent string) sealResponse {
	return openapi.SealAgentDraft404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(agent)}
}

func sealInvalid(code Code, detail string) sealResponse {
	return openapi.SealAgentDraft400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			refusal(http.StatusBadRequest, code, "Not sealable", detail)),
	}
}

func sealStale(current domain.Revision) sealResponse {
	stale, _ := staleFrom(&domain.StaleError{Current: current})
	return openapi.SealAgentDraft412ApplicationProblemPlusJSONResponse{PreconditionFailedApplicationProblemPlusJSONResponse: stale}
}

func sealConflict(code Code, title string, err error) sealResponse {
	return openapi.SealAgentDraft409ApplicationProblemPlusJSONResponse(refusal(http.StatusConflict, code, title, err.Error()))
}
