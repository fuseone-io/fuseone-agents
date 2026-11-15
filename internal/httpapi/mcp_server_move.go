package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// MCPServerMover moves a server out of the installation (NT-019), declared
// here by the consumer.
type MCPServerMover interface {
	MoveMCPServer(ctx context.Context, m admin.MCPServerMove) (domain.Revision, error)
}

/*
MoveMCPServer stores a server of the installation at a company or area
(NT-019). The platform's administration alone moves one: it takes the server
from every other scope. It is made against the revision read, so a request
without If-Match is refused before anything else is asked.
*/
func (s *Server) MoveMCPServer(ctx context.Context, req openapi.MoveMCPServerRequestObject) (openapi.MoveMCPServerResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermProviderWrite); resp != nil {
		return openapi.MoveMCPServer403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: *resp}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	expect, err := domain.ParseIfMatch(req.Params.IfMatch)
	if err != nil {
		return moveBadRequest(invalid(err.Error())), nil
	}
	if !expect.Present() {
		return openapi.MoveMCPServer428ApplicationProblemPlusJSONResponse(refusal(http.StatusPreconditionRequired,
			CodePreconditionRequired, "Precondition required", "send If-Match with the revision read")), nil
	}
	to, err := moveTarget(req.Body)
	if err != nil {
		return moveBadRequest(invalid(err.Error())), nil
	}
	caller, _ := auth.PrincipalFrom(ctx)
	revision, err := s.integrations.MoveMCPServer(ctx, admin.MCPServerMove{
		By: caller.ID, Audit: adminScope, Name: req.Name, To: to, Expect: expect,
		Strand: req.Body.Strand != nil && *req.Body.Strand,
	})
	if err != nil {
		return moveRefusal(req.Name, err)
	}
	return openapi.MoveMCPServer204Response{Headers: openapi.MoveMCPServer204ResponseHeaders{ETag: string(revision)}}, nil
}

// moveTarget is a company or one of its areas, written as stored. realScope
// refuses the installation's `*`, which a server only leaves.
func moveTarget(body *openapi.MoveMCPServerJSONRequestBody) (domain.Scope, error) {
	if body == nil || body.Company == "" {
		return domain.Scope{}, errors.New("company is required")
	}
	to := domain.Scope{Company: domain.CompanyID(body.Company)}
	if body.Area != nil {
		to.Area = domain.AreaID(*body.Area)
	}
	if err := realScope(to); err != nil {
		return domain.Scope{}, err
	}
	return to, nil
}

func moveRefusal(name string, err error) (openapi.MoveMCPServerResponseObject, error) {
	var stranded *admin.MoveStrandsAgents
	var egress *admin.EgressRefusal
	if stale, ok := staleFrom(err); ok {
		return openapi.MoveMCPServer412ApplicationProblemPlusJSONResponse{PreconditionFailedApplicationProblemPlusJSONResponse: stale}, nil
	}
	switch {
	case errors.As(err, &stranded):
		return moveConflict(CodeMoveStrandsAgents, "published agents outside the target name its tools", strandedOf(stranded)), nil
	case errors.Is(err, admin.ErrMoveNotAtInstallation):
		return moveConflict(CodeServerNotAtInstallation, err.Error(), nil), nil
	case errors.As(err, &egress):
		return moveBadRequest(egressNotAllowed(egress.Origin)), nil
	case errors.Is(err, admin.ErrMoveTargetUnregistered), errors.Is(err, admin.ErrMoveLocalServer),
		errors.Is(err, admin.ErrEgressAddressInvalid):
		return moveBadRequest(invalid(err.Error())), nil
	case errors.Is(err, admin.ErrMCPServerUnknown):
		return openapi.MoveMCPServer404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(name)}, nil
	}
	return nil, err
}

func moveBadRequest(p openapi.Problem) openapi.MoveMCPServerResponseObject {
	return openapi.MoveMCPServer400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(p),
	}
}

func moveConflict(code Code, detail string, agents *[]openapi.StrandedAgent) openapi.MoveMCPServerResponseObject {
	p := refusal(http.StatusConflict, code, "Conflict", detail)
	return openapi.MoveMCPServer409ApplicationProblemPlusJSONResponse{
		Type: p.Type, Title: p.Title, Status: p.Status, Detail: p.Detail, Agents: agents,
	}
}

func strandedOf(e *admin.MoveStrandsAgents) *[]openapi.StrandedAgent {
	out := make([]openapi.StrandedAgent, 0, len(e.Agents))
	for _, a := range e.Agents {
		out = append(out, openapi.StrandedAgent{AgentId: string(a.Agent), Company: string(a.Scope.Company), Area: string(a.Scope.Area)})
	}
	return &out
}
