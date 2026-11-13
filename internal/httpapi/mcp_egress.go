package httpapi

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
The platform's egress policies for MCP servers outside the installation
(NT-019, decided 2026-11-13).

Every door here asks for configuring the platform itself (configurer), never
a scope of the caller's: an area's administrator is whom the list holds, and
may neither read how it is drawn nor widen it.
*/

func (s *Server) ListMCPEgressPolicies(ctx context.Context, _ openapi.ListMCPEgressPoliciesRequestObject) (openapi.ListMCPEgressPoliciesResponseObject, error) {
	if _, forbidden := s.configurer(ctx); forbidden != nil {
		return openapi.ListMCPEgressPolicies403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: *forbidden}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	policies, err := s.integrations.MCPEgressPolicies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]openapi.MCPEgressPolicy, 0, len(policies))
	for _, p := range policies {
		item := openapi.MCPEgressPolicy{
			Company: string(p.Scope.Company), Origins: p.Origins,
			UpdatedBy: p.UpdatedBy, UpdatedAt: p.UpdatedAt,
		}
		if p.Scope.Area != "" {
			item.Area = ptr(string(p.Scope.Area))
		}
		out = append(out, item)
	}
	return openapi.ListMCPEgressPolicies200JSONResponse{Policies: out}, nil
}

func (s *Server) PutMCPEgressPolicy(ctx context.Context, req openapi.PutMCPEgressPolicyRequestObject) (openapi.PutMCPEgressPolicyResponseObject, error) {
	caller, forbidden := s.configurer(ctx)
	if forbidden != nil {
		return openapi.PutMCPEgressPolicy403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: *forbidden}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	if req.Body == nil {
		return openapi.PutMCPEgressPolicy400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid("a policy names a company and its origins")),
		}, nil
	}
	err := s.integrations.PutMCPEgressPolicy(ctx, caller, adminScope, admin.MCPEgressPolicy{
		Scope: egressScope(req.Body.Company, req.Body.Area), Origins: req.Body.Origins,
	})
	if errors.Is(err, admin.ErrEgressPolicyInvalid) {
		return openapi.PutMCPEgressPolicy400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return openapi.PutMCPEgressPolicy204Response{}, nil
}

func (s *Server) DeleteMCPEgressPolicy(ctx context.Context, req openapi.DeleteMCPEgressPolicyRequestObject) (openapi.DeleteMCPEgressPolicyResponseObject, error) {
	caller, forbidden := s.configurer(ctx)
	if forbidden != nil {
		return openapi.DeleteMCPEgressPolicy403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: *forbidden}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	err := s.integrations.DeleteMCPEgressPolicy(ctx, caller, adminScope, egressScope(req.Params.Company, req.Params.Area))
	if errors.Is(err, admin.ErrEgressPolicyInvalid) {
		return openapi.DeleteMCPEgressPolicy400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return openapi.DeleteMCPEgressPolicy204Response{}, nil
}

func egressScope(company string, area *string) domain.Scope {
	scope := domain.Scope{Company: domain.CompanyID(company)}
	if area != nil {
		scope.Area = domain.AreaID(*area)
	}
	return scope
}
