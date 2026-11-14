package httpapi

import (
	"context"
	"fmt"
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
				invalid("scope must be installation, a company, or company/area")),
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
				notStored(err.Error())),
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

	scope, ok := parseBudgetShape(req.Scope)
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
// installation, and an area with no company is not a scope. And a real one,
// written once (NT-019): a company or area in another form (spaces around a
// part, upper case, a further slash, the installation's `*`) is refused, never
// folded, as the connector and MCP doors refuse it, so the ceiling governs the
// scope the caller named and not a lookalike no agent belongs to.
func parseBudgetScope(raw string) (domain.Scope, bool) {
	scope, ok := parseBudgetShape(raw)
	if !ok || (scope != domain.Scope{} && realScope(scope) != nil) {
		return domain.Scope{}, false
	}
	return scope, true
}

// parseBudgetShape reads a path by its shape alone. Removal takes it, so a
// ceiling stored in another form before scopes were checked can still be
// removed: refused here it would answer 204 and stay.
func parseBudgetShape(raw string) (domain.Scope, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "installation" {
		return domain.Scope{}, true
	}
	company, area, hasArea := strings.Cut(raw, "/")
	if company == "" || (hasArea && area == "") {
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
