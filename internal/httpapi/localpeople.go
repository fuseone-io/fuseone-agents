package httpapi

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Accounts that do not need an identity provider.

Beside the provider and never instead of it. Where a customer has one, that is
how people arrive — and the hole this fills is that until one is configured an
installation has exactly one account, the one the setup token created. Four
roles that exist to hold an author and an approver apart cannot be shown with
a single account, and an administrator who lost that session had no door left.
*/

// Accounts is local sign-in, declared here by the consumer.
type Accounts interface {
	Create(ctx context.Context, username, display, email, password, by string) (string, error)
	SetPassword(ctx context.Context, principalID, password string) error
	SetUsername(ctx context.Context, principalID, username string) error
}

// WithAccounts wires accounts that sign in with a password.
func (s *Server) WithAccounts(accounts Accounts) *Server {
	s.accounts = accounts
	return s
}

func (s *Server) CreateLocalPerson(
	ctx context.Context, req openapi.CreateLocalPersonRequestObject,
) (openapi.CreateLocalPersonResponseObject, error) {
	if err := auth.Require(ctx, domain.PermIdentityWrite, adminScope); err != nil {
		return openapi.CreateLocalPerson403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermIdentityWrite, adminScope),
		}, nil
	}
	if s.accounts == nil || req.Body == nil {
		return nil, errNoAdministration
	}

	id, err := s.accounts.Create(ctx,
		req.Body.Username, valueOr(req.Body.Display), valueOr(req.Body.Email),
		req.Body.Password, string(callerOf(ctx)))
	switch {
	case errors.Is(err, auth.ErrUsernameTaken):
		return openapi.CreateLocalPerson409ApplicationProblemPlusJSONResponse(
			conflicted(err.Error())), nil
	case err != nil:
		return openapi.CreateLocalPerson400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}

	// Holding nothing. Granting is the next act and a separate entry in the
	// trail: creating somebody and deciding what they may do are two
	// decisions, and an auditor reading one should not have to infer the other.
	return openapi.CreateLocalPerson201JSONResponse(openapi.Person{
		Id: id, Kind: openapi.PersonKind(domain.PrincipalUser),
		Display:  displayOr(valueOr(req.Body.Display), req.Body.Username),
		Username: ptr(req.Body.Username),
	}), nil
}

func (s *Server) SetPassword(
	ctx context.Context, req openapi.SetPasswordRequestObject,
) (openapi.SetPasswordResponseObject, error) {
	// Somebody may always set their own, which is what stops "change your
	// password" needing the authority to administer everybody else's.
	if string(callerOf(ctx)) != req.PrincipalId {
		if err := auth.Require(ctx, domain.PermIdentityWrite, adminScope); err != nil {
			return openapi.SetPassword403ApplicationProblemPlusJSONResponse{
				ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermIdentityWrite, adminScope),
			}, nil
		}
	}
	if s.accounts == nil || req.Body == nil {
		return nil, errNoAdministration
	}

	// The handle first: a password on an account nobody can name is a
	// credential with no way to present it.
	if req.Body.Username != nil && *req.Body.Username != "" {
		if err := s.accounts.SetUsername(ctx, req.PrincipalId, *req.Body.Username); err != nil {
			return passwordRefused(err)
		}
	}
	if err := s.accounts.SetPassword(ctx, req.PrincipalId, req.Body.Password); err != nil {
		return passwordRefused(err)
	}
	return openapi.SetPassword204Response{}, nil
}

func passwordRefused(err error) (openapi.SetPasswordResponseObject, error) {
	if errors.Is(err, auth.ErrUsernameTaken) {
		return openapi.SetPassword409ApplicationProblemPlusJSONResponse(
			conflicted(err.Error())), nil
	}
	return openapi.SetPassword400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			invalid(err.Error())),
	}, nil
}

func displayOr(display, username string) string {
	if display == "" {
		return username
	}
	return display
}
