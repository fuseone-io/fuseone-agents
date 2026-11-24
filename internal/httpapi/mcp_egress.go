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
			UpdatedBy: p.UpdatedBy, UpdatedAt: p.UpdatedAt, Etag: string(p.Revision),
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
	expect, create, refused := egressPrecondition(req.Params.IfMatch, req.Params.IfNoneMatch)
	if refused != "" {
		return openapi.PutMCPEgressPolicy400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(refused)),
		}, nil
	}
	revision, err := s.integrations.WriteMCPEgressPolicy(ctx, admin.EgressPolicyWrite{
		By: caller, Audit: adminScope, Expect: expect, Create: create,
		Policy: admin.MCPEgressPolicy{Scope: egressScope(req.Body.Company, req.Body.Area), Origins: req.Body.Origins},
	})
	if stale, ok := staleFrom(err); ok {
		return openapi.PutMCPEgressPolicy412ApplicationProblemPlusJSONResponse{PreconditionFailedApplicationProblemPlusJSONResponse: stale}, nil
	}
	if errors.Is(err, admin.ErrEgressPolicyInvalid) {
		return openapi.PutMCPEgressPolicy400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return openapi.PutMCPEgressPolicy204Response{Headers: openapi.PutMCPEgressPolicy204ResponseHeaders{ETag: string(revision)}}, nil
}

// egressPrecondition reads what a policy write says it saw: an If-Match to
// edit, or If-None-Match: * to create, never both. Neither replaces.
func egressPrecondition(ifMatch, ifNoneMatch *string) (domain.IfMatch, bool, string) {
	if ifMatch != nil && ifNoneMatch != nil {
		return domain.IfMatch{}, false, "send If-Match to change a policy or If-None-Match: * to create one, not both"
	}
	expect, err := domain.ParseIfMatch(ifMatch)
	if err != nil {
		return domain.IfMatch{}, false, err.Error()
	}
	absent, err := domain.ParseIfNoneMatch(ifNoneMatch)
	if err != nil {
		return domain.IfMatch{}, false, err.Error()
	}
	return expect, absent, ""
}

func (s *Server) DeleteMCPEgressPolicy(ctx context.Context, req openapi.DeleteMCPEgressPolicyRequestObject) (openapi.DeleteMCPEgressPolicyResponseObject, error) {
	caller, forbidden := s.configurer(ctx)
	if forbidden != nil {
		return openapi.DeleteMCPEgressPolicy403ApplicationProblemPlusJSONResponse{ForbiddenApplicationProblemPlusJSONResponse: *forbidden}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	expect, err := domain.ParseIfMatch(req.Params.IfMatch)
	if err != nil {
		return openapi.DeleteMCPEgressPolicy400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	}
	err = s.integrations.RemoveMCPEgressPolicy(ctx, admin.EgressPolicyRemoval{
		By: caller, Audit: adminScope, At: egressScope(req.Params.Company, req.Params.Area), Expect: expect,
	})
	if stale, ok := staleFrom(err); ok {
		return openapi.DeleteMCPEgressPolicy412ApplicationProblemPlusJSONResponse{PreconditionFailedApplicationProblemPlusJSONResponse: stale}, nil
	}
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
