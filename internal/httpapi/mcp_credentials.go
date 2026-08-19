package httpapi

import (
	"context"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

func (s *Server) ListMCPUserCredentials(
	ctx context.Context, _ openapi.ListMCPUserCredentialsRequestObject,
) (openapi.ListMCPUserCredentialsResponseObject, error) {
	caller, refused := s.mcpCredentialCaller(ctx)
	if refused != nil {
		return openapi.ListMCPUserCredentials403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	body := openapi.ListMCPUserCredentials200JSONResponse{
		Items: []openapi.MCPUserCredential{},
	}
	if s.integrations == nil {
		return body, nil
	}

	listed, err := s.integrations.MCPPersonalCredentials(ctx, caller.ID)
	if err != nil {
		return nil, err
	}
	for _, credential := range listed {
		body.Items = append(body.Items, mcpUserCredentialOf(credential))
	}
	return body, nil
}

func (s *Server) PutMCPUserCredential(
	ctx context.Context, req openapi.PutMCPUserCredentialRequestObject,
) (openapi.PutMCPUserCredentialResponseObject, error) {
	caller, refused := s.mcpCredentialCaller(ctx)
	if refused != nil {
		return openapi.PutMCPUserCredential403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	if req.Body == nil {
		return openapi.PutMCPUserCredential400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("credential body is required")),
		}, nil
	}

	creds := domain.MCPCredentialPatch{
		Token:   req.Body.Token,
		Headers: stringMapOrNil(req.Body.Headers),
		OAuth:   oauthGrantFromRequest(req.Body.Oauth),
	}
	if err := s.integrations.PutMCPPersonalCredential(ctx,
		caller.ID, firstVisibleScope(caller), req.Name, creds); err != nil {
		return openapi.PutMCPUserCredential400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	return openapi.PutMCPUserCredential204Response{}, nil
}

func (s *Server) DeleteMCPUserCredential(
	ctx context.Context, req openapi.DeleteMCPUserCredentialRequestObject,
) (openapi.DeleteMCPUserCredentialResponseObject, error) {
	caller, refused := s.mcpCredentialCaller(ctx)
	if refused != nil {
		return openapi.DeleteMCPUserCredential403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	if s.integrations == nil {
		return nil, errNoAdministration
	}
	if err := s.integrations.DeleteMCPPersonalCredential(ctx,
		caller.ID, firstVisibleScope(caller), req.Name); err != nil {
		return nil, err
	}
	return openapi.DeleteMCPUserCredential204Response{}, nil
}

func (s *Server) mcpCredentialCaller(
	ctx context.Context,
) (domain.Principal, *openapi.ForbiddenApplicationProblemPlusJSONResponse) {
	caller, ok := auth.PrincipalFrom(ctx)
	if !ok || !caller.CanAnywhere(domain.PermToolRead) {
		body := forbidden(domain.PermToolRead, domain.Scope{})
		return domain.Principal{}, &body
	}
	return caller, nil
}

func firstVisibleScope(caller domain.Principal) domain.Scope {
	for _, grant := range caller.Grants {
		if grant.Role.Allows(domain.PermToolRead) {
			return grant.Scope
		}
	}
	return domain.Scope{}
}

func mcpUserCredentialOf(credential domain.MCPPersonalCredential) openapi.MCPUserCredential {
	return openapi.MCPUserCredential{
		Server: credential.Server, HasCredential: credential.HasSecret,
		HasHeaders: credential.HasHeaders, HasOAuth: credential.HasOAuth,
		UpdatedBy: ptr(credential.UpdatedBy), UpdatedAt: ptr(credential.UpdatedAt),
	}
}
