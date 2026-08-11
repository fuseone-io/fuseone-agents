package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Ceilings is what a scope may spend, declared here by the consumer.
type Ceilings interface {
	List(ctx context.Context) ([]domain.ScopeBudget, error)
	Put(ctx context.Context, by domain.UserID, budget domain.ScopeBudget) error
	Delete(ctx context.Context, by domain.UserID, scope domain.Scope) error
}

func (s *Server) ListBudgets(ctx context.Context, _ openapi.ListBudgetsRequestObject) (openapi.ListBudgetsResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermBudgetWrite); resp != nil {
		return openapi.ListBudgets403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.ceilings == nil {
		return openapi.ListBudgets200JSONResponse{Items: []openapi.ScopeBudget{}}, nil
	}

	configured, err := s.ceilings.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list budgets: %w", err)
	}

	items := make([]openapi.ScopeBudget, 0, len(configured))
	for _, b := range configured {
		items = append(items, budgetFrom(b))
	}
	return openapi.ListBudgets200JSONResponse{Items: items}, nil
}

func (s *Server) PutBudget(ctx context.Context, req openapi.PutBudgetRequestObject) (openapi.PutBudgetResponseObject, error) {
	caller, forbidden := s.budgetSetter(ctx)
	if forbidden != nil {
		return openapi.PutBudget403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.ceilings == nil {
		return nil, errNoAdministration
	}

	scope, ok := parseBudgetScope(req.Scope)
	if !ok {
		return openapi.PutBudget400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Escopo inválido",
					"use installation, uma empresa, ou empresa/área")),
		}, nil
	}

	budget := domain.ScopeBudget{
		Scope: scope, Period: domain.Period(req.Body.Period), Enabled: true,
		Budget: domain.Budget{
			Micros:      valueOr(req.Body.Micros),
			Tokens:      valueOr(req.Body.Tokens),
			ToolCalls:   valueOr(req.Body.ToolCalls),
			Steps:       valueOr(req.Body.Steps),
			WallClockMS: valueOr(req.Body.WallClockMs),
		},
	}
	if req.Body.Enabled != nil {
		budget.Enabled = *req.Body.Enabled
	}

	if err := s.ceilings.Put(ctx, caller, budget); err != nil {
		return openapi.PutBudget400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Não foi possível definir o teto", err.Error())),
		}, nil
	}
	return openapi.PutBudget204Response{}, nil
}

func (s *Server) DeleteBudget(ctx context.Context, req openapi.DeleteBudgetRequestObject) (openapi.DeleteBudgetResponseObject, error) {
	caller, forbidden := s.budgetSetter(ctx)
	if forbidden != nil {
		return openapi.DeleteBudget403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *forbidden,
		}, nil
	}
	if s.ceilings == nil {
		return nil, errNoAdministration
	}

	scope, ok := parseBudgetScope(req.Scope)
	if !ok {
		return openapi.DeleteBudget204Response{}, nil
	}
	if err := s.ceilings.Delete(ctx, caller, scope); err != nil {
		return nil, err
	}
	return openapi.DeleteBudget204Response{}, nil
}

func (s *Server) budgetSetter(ctx context.Context) (domain.UserID, *openapi.ForbiddenApplicationProblemPlusJSONResponse) {
	if resp := s.refuse(ctx, domain.PermBudgetWrite); resp != nil {
		return "", resp
	}
	caller, _ := auth.PrincipalFrom(ctx)
	return caller.ID, nil
}

// parseBudgetScope reads the three shapes a ceiling can be set on.
//
// A closed set rather than free text: "installation" is not a company called
// installation, and an area with no company is not a scope.
func parseBudgetScope(raw string) (domain.Scope, bool) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "" || raw == "installation":
		return domain.Scope{}, true
	case !strings.Contains(raw, "/"):
		return domain.Scope{Company: domain.CompanyID(raw)}, true
	}

	company, area, _ := strings.Cut(raw, "/")
	if company == "" || area == "" {
		return domain.Scope{}, false
	}
	return domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}, true
}

func budgetFrom(b domain.ScopeBudget) openapi.ScopeBudget {
	out := openapi.ScopeBudget{
		ScopeKind: openapi.ScopeBudgetScopeKind(b.ScopeKind),
		Period:    openapi.ScopeBudgetPeriod(b.Period),
		Enabled:   b.Enabled,
		Scope:     &openapi.Scope{Company: string(b.Scope.Company), Area: string(b.Scope.Area)},
	}
	if b.Budget.Micros != 0 {
		out.Micros = ptr(b.Budget.Micros)
	}
	if b.Budget.Tokens != 0 {
		out.Tokens = ptr(b.Budget.Tokens)
	}
	if b.Budget.ToolCalls != 0 {
		out.ToolCalls = ptr(b.Budget.ToolCalls)
	}
	if b.Budget.Steps != 0 {
		out.Steps = ptr(b.Budget.Steps)
	}
	if b.Budget.WallClockMS != 0 {
		out.WallClockMs = ptr(b.Budget.WallClockMS)
	}
	if b.UpdatedBy != "" {
		out.UpdatedBy = ptr(string(b.UpdatedBy))
	}
	if !b.UpdatedAt.IsZero() {
		out.UpdatedAt = ptr(b.UpdatedAt)
	}
	return out
}
