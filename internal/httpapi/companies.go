package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
The companies an installation governs.

Every check here is against domain.Installation and never against a company.
Held inside one, this authority would let that company's administrator mint
another and grant themselves in it, which is not a tightening anybody would
notice — so the scope is the point, not the role.
*/

// CompanyAdmin is the registry of companies, declared here by the consumer.
type CompanyAdmin interface {
	List(ctx context.Context) ([]admin.Company, error)
	Create(ctx context.Context, id domain.CompanyID, label string, by domain.UserID) (admin.Company, error)
	Rename(ctx context.Context, id domain.CompanyID, label string, by domain.UserID) error
	Archive(ctx context.Context, id domain.CompanyID, by domain.UserID) error
	Restore(ctx context.Context, id domain.CompanyID, by domain.UserID) error
}

// WithCompanies wires the registry.
func (s *Server) WithCompanies(companies CompanyAdmin) *Server {
	s.companies = companies
	return s
}

// governs checks the one authority above every company, and answers with the
// caller when they hold it.
func (s *Server) governs(ctx context.Context) (domain.UserID, *openapi.ForbiddenApplicationProblemPlusJSONResponse) {
	everywhere := domain.Scope{Company: domain.Installation}
	if err := auth.Require(ctx, domain.PermCompanyWrite, everywhere); err != nil {
		body := forbidden(domain.PermCompanyWrite, everywhere)
		return "", &body
	}
	return callerOf(ctx), nil
}

func (s *Server) ListCompanies(
	ctx context.Context, _ openapi.ListCompaniesRequestObject,
) (openapi.ListCompaniesResponseObject, error) {
	if _, resp := s.governs(ctx); resp != nil {
		return openapi.ListCompanies403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.companies == nil {
		return openapi.ListCompanies200JSONResponse{Items: []openapi.Company{}}, nil
	}

	registered, err := s.companies.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list companies: %w", err)
	}

	items := make([]openapi.Company, 0, len(registered))
	for _, company := range registered {
		items = append(items, companyFrom(company))
	}
	return openapi.ListCompanies200JSONResponse{Items: items}, nil
}

func (s *Server) CreateCompany(
	ctx context.Context, req openapi.CreateCompanyRequestObject,
) (openapi.CreateCompanyResponseObject, error) {
	caller, resp := s.governs(ctx)
	if resp != nil {
		return openapi.CreateCompany403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.companies == nil || req.Body == nil {
		return nil, errNoAdministration
	}

	created, err := s.companies.Create(ctx,
		domain.CompanyID(req.Body.Id), valueOr(req.Body.Label), caller)
	switch {
	case errors.Is(err, admin.ErrCompanyExists):
		return openapi.CreateCompany409ApplicationProblemPlusJSONResponse(
			conflicted(err.Error())), nil
	case err != nil:
		return openapi.CreateCompany400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	return openapi.CreateCompany201JSONResponse(companyFrom(created)), nil
}

func (s *Server) UpdateCompany(
	ctx context.Context, req openapi.UpdateCompanyRequestObject,
) (openapi.UpdateCompanyResponseObject, error) {
	caller, resp := s.governs(ctx)
	if resp != nil {
		return openapi.UpdateCompany403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.companies == nil || req.Body == nil {
		return nil, errNoAdministration
	}

	id := domain.CompanyID(req.Company)
	if err := s.change(ctx, id, req.Body, caller); err != nil {
		if errors.Is(err, admin.ErrNoSuchCompany) {
			return openapi.UpdateCompany404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound(req.Company),
			}, nil
		}
		return openapi.UpdateCompany400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	return openapi.UpdateCompany204Response{}, nil
}

// change applies whichever halves the request carried.
//
// Two separate acts rather than one write, because they are two entries in the
// trail: renaming a company and withdrawing it are different decisions and an
// auditor reading one should not have to infer which happened.
func (s *Server) change(
	ctx context.Context, id domain.CompanyID,
	body *openapi.UpdateCompanyJSONRequestBody, by domain.UserID,
) error {
	if body.Label != nil {
		if err := s.companies.Rename(ctx, id, *body.Label, by); err != nil {
			return err
		}
	}
	if body.Archived == nil {
		return nil
	}
	if *body.Archived {
		return s.companies.Archive(ctx, id, by)
	}
	return s.companies.Restore(ctx, id, by)
}

func companyFrom(c admin.Company) openapi.Company {
	out := openapi.Company{
		Id: string(c.ID), Label: c.Label, Areas: c.Areas, Archived: c.Archived,
	}
	if !c.CreatedAt.IsZero() {
		out.CreatedAt = &c.CreatedAt
	}
	if c.CreatedBy != "" {
		out.CreatedBy = ptr(string(c.CreatedBy))
	}
	return out
}
