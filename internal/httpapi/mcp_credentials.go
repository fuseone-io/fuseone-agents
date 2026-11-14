package httpapi

import (
	"context"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// PersonalCredentials is what the doors of a person's own MCP credentials
// need, declared here by the consumer.
type PersonalCredentials interface {
	MCPPersonalCredentials(ctx context.Context, principal domain.UserID) ([]domain.MCPPersonalCredential, error)
	MCPPersonalCredentialPresence(ctx context.Context, principal domain.UserID, servers []string) (map[string]bool, error)
	// MCPPersonalCredentialServers are the remote servers the given scopes
	// reach, by the write's rule, with no address (NT-019).
	MCPPersonalCredentialServers(ctx context.Context, visible []domain.Scope) ([]domain.MCPCredentialServer, error)
	PutMCPPersonalCredential(ctx context.Context, by domain.UserID, scope domain.Scope, name string, creds domain.MCPCredentialPatch) error
	// WriteMCPPersonalCredential checks, under the server's row, that the
	// person's scopes reach the server (NT-019).
	WriteMCPPersonalCredential(ctx context.Context, w admin.PersonalCredentialWrite) error
	DeleteMCPPersonalCredential(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error
}

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
	// The servers they may hold one for, asked by where they read tools: an
	// area's author cannot list the servers' configuration (NT-019).
	servers, err := s.integrations.MCPPersonalCredentialServers(ctx, caller.ScopesFor(domain.PermToolRead))
	if err != nil {
		return nil, err
	}
	offered := make([]openapi.MCPCredentialServer, 0, len(servers))
	for _, server := range servers {
		offered = append(offered, mcpCredentialServerOf(server))
	}
	body.Servers = &offered
	return body, nil
}

func mcpCredentialServerOf(server domain.MCPCredentialServer) openapi.MCPCredentialServer {
	out := openapi.MCPCredentialServer{Name: server.Name, Shared: server.Shared,
		ScopeKind: openapi.MCPCredentialServerScopeKindInstallation}
	if server.Scope.Company == "" {
		return out
	}
	out.ScopeKind, out.Company = openapi.MCPCredentialServerScopeKindCompany, ptr(string(server.Scope.Company))
	if server.Scope.Area != "" {
		out.ScopeKind, out.Area = openapi.MCPCredentialServerScopeKindArea, ptr(string(server.Scope.Area))
	}
	return out
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
	// Every scope the person reads tools in: a server their scopes do not
	// reach is one they may not hold a credential for, and the check is made
	// under the server's row when the credential is written (NT-019).
	if err := s.integrations.WriteMCPPersonalCredential(ctx, admin.PersonalCredentialWrite{
		By: caller.ID, Scope: firstVisibleScope(caller), Visible: caller.ScopesFor(domain.PermToolRead),
		Server: req.Name, Given: creds,
	}); err != nil {
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
