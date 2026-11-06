package httpapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// ServiceAccounts is where machine identities are kept, declared by the consumer.
type ServiceAccounts interface {
	ServiceAccounts(ctx context.Context) ([]domain.ServiceAccount, error)
	CreateServiceAccount(ctx context.Context, name, description, by string) (domain.ServiceAccount, error)
	SetServiceAccountDisabled(ctx context.Context, id string, disabled bool, by string, now time.Time) error
	ServiceTokens(ctx context.Context, account string) ([]domain.ServiceToken, error)
	IssueServiceToken(ctx context.Context, account, name, by string, expires, now time.Time) (auth.IssuedToken, error)
	RevokeServiceToken(ctx context.Context, account, token, by string, now time.Time) error
}

// WithServiceAccounts wires machine identities.
func (s *Server) WithServiceAccounts(accounts ServiceAccounts) *Server {
	s.serviceAccounts = accounts
	return s
}

// mayManageServiceAccounts is identity:write, the same as people. A service account
// can never hold it, so a token cannot issue a token, whatever role it was given.
func (s *Server) mayManageServiceAccounts(ctx context.Context) (*openapi.ForbiddenApplicationProblemPlusJSONResponse, error) {
	if err := auth.Require(ctx, domain.PermIdentityWrite, identityScope); err != nil {
		denied := forbidden(domain.PermIdentityWrite, identityScope)
		return &denied, nil
	}
	if s.serviceAccounts == nil {
		return nil, errNoAdministration
	}
	return nil, nil
}

func (s *Server) ListServiceAccounts(
	ctx context.Context, _ openapi.ListServiceAccountsRequestObject,
) (openapi.ListServiceAccountsResponseObject, error) {
	if err := auth.Require(ctx, domain.PermIdentityWrite, identityScope); err != nil {
		return openapi.ListServiceAccounts403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermIdentityWrite, identityScope),
		}, nil
	}
	items := []openapi.ServiceAccount{}
	if s.serviceAccounts != nil {
		found, err := s.serviceAccounts.ServiceAccounts(ctx)
		if err != nil {
			return nil, fmt.Errorf("list service accounts: %w", err)
		}
		for _, account := range found {
			items = append(items, toServiceAccount(account))
		}
	}
	return openapi.ListServiceAccounts200JSONResponse{Items: items}, nil
}

func (s *Server) CreateServiceAccount(
	ctx context.Context, req openapi.CreateServiceAccountRequestObject,
) (openapi.CreateServiceAccountResponseObject, error) {
	denied, err := s.mayManageServiceAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if denied != nil {
		return openapi.CreateServiceAccount403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *denied,
		}, nil
	}
	created, err := s.serviceAccounts.CreateServiceAccount(ctx,
		req.Body.Name, valueOr(req.Body.Description), string(callerOf(ctx)))
	switch {
	case errors.Is(err, auth.ErrServiceAccountTaken):
		return openapi.CreateServiceAccount409ApplicationProblemPlusJSONResponse(conflicted(err.Error())), nil
	case errors.Is(err, auth.ErrServiceAccountName):
		return openapi.CreateServiceAccount400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	case err != nil:
		return nil, fmt.Errorf("create service account: %w", err)
	}
	return openapi.CreateServiceAccount201JSONResponse(toServiceAccount(created)), nil
}

func (s *Server) SetServiceAccountDisabled(
	ctx context.Context, req openapi.SetServiceAccountDisabledRequestObject,
) (openapi.SetServiceAccountDisabledResponseObject, error) {
	denied, err := s.mayManageServiceAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if denied != nil {
		return openapi.SetServiceAccountDisabled403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *denied,
		}, nil
	}
	err = s.serviceAccounts.SetServiceAccountDisabled(ctx, req.AccountId, req.Body.Disabled,
		string(callerOf(ctx)), clockOr(s.clock).Now())
	if errors.Is(err, auth.ErrNoServiceAccount) {
		return openapi.SetServiceAccountDisabled404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.AccountId),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("set service account state: %w", err)
	}
	return openapi.SetServiceAccountDisabled204Response{}, nil
}

func (s *Server) ListServiceTokens(
	ctx context.Context, req openapi.ListServiceTokensRequestObject,
) (openapi.ListServiceTokensResponseObject, error) {
	denied, err := s.mayManageServiceAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if denied != nil {
		return openapi.ListServiceTokens403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *denied,
		}, nil
	}
	found, err := s.serviceAccounts.ServiceTokens(ctx, req.AccountId)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	items := make([]openapi.ServiceToken, 0, len(found))
	for _, token := range found {
		items = append(items, toServiceToken(token))
	}
	return openapi.ListServiceTokens200JSONResponse{Items: items}, nil
}

func (s *Server) IssueServiceToken(
	ctx context.Context, req openapi.IssueServiceTokenRequestObject,
) (openapi.IssueServiceTokenResponseObject, error) {
	denied, err := s.mayManageServiceAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if denied != nil {
		return openapi.IssueServiceToken403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *denied,
		}, nil
	}
	issued, err := s.serviceAccounts.IssueServiceToken(ctx, req.AccountId, req.Body.Name,
		string(callerOf(ctx)), req.Body.ExpiresAt, clockOr(s.clock).Now())
	switch {
	case errors.Is(err, auth.ErrTokenExpiry):
		return openapi.IssueServiceToken400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}, nil
	case errors.Is(err, auth.ErrNoServiceAccount):
		return openapi.IssueServiceToken404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.AccountId),
		}, nil
	case errors.Is(err, auth.ErrServiceAccountDisabled):
		return openapi.IssueServiceToken409ApplicationProblemPlusJSONResponse(conflicted(err.Error())), nil
	case err != nil:
		return nil, fmt.Errorf("issue token: %w", err)
	}
	token := toServiceToken(issued.ServiceToken)
	return openapi.IssueServiceToken201JSONResponse{
		Id: token.Id, Name: token.Name, CreatedBy: token.CreatedBy, CreatedAt: token.CreatedAt,
		ExpiresAt: token.ExpiresAt, Secret: issued.Secret,
	}, nil
}

func (s *Server) RevokeServiceToken(
	ctx context.Context, req openapi.RevokeServiceTokenRequestObject,
) (openapi.RevokeServiceTokenResponseObject, error) {
	denied, err := s.mayManageServiceAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if denied != nil {
		return openapi.RevokeServiceToken403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *denied,
		}, nil
	}
	err = s.serviceAccounts.RevokeServiceToken(ctx, req.AccountId, req.TokenId,
		string(callerOf(ctx)), clockOr(s.clock).Now())
	if errors.Is(err, auth.ErrNoServiceToken) {
		return openapi.RevokeServiceToken404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.TokenId),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("revoke token: %w", err)
	}
	return openapi.RevokeServiceToken204Response{}, nil
}

func toServiceAccount(account domain.ServiceAccount) openapi.ServiceAccount {
	grants := make([]openapi.GrantInput, 0, len(account.Grants))
	for _, g := range account.Grants {
		grants = append(grants, openapi.GrantInput{
			Company: string(g.Scope.Company), Area: string(g.Scope.Area),
			Role: openapi.GrantInputRole(g.Role),
		})
	}
	return openapi.ServiceAccount{
		Id: account.ID, Name: account.Name, Description: someString(account.Description),
		Disabled: account.Disabled, CreatedAt: account.CreatedAt, Grants: grants,
	}
}

func toServiceToken(token domain.ServiceToken) openapi.ServiceToken {
	return openapi.ServiceToken{
		Id: token.ID, Name: token.Name, CreatedBy: token.CreatedBy, CreatedAt: token.CreatedAt,
		ExpiresAt: token.ExpiresAt, LastUsedAt: token.LastUsedAt, RevokedAt: token.RevokedAt,
	}
}
