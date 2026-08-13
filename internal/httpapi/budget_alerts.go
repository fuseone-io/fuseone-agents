package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Marks are the budget thresholds each scope has crossed, declared here by the
// consumer (PRD FO-05).
type Marks interface {
	Announced(ctx context.Context) (map[string]domain.BudgetMark, error)
}

/*
ListBudgetAlerts is which scopes are running out of money.

Narrowed to what the caller may see, like every other read: an author in one
area has no business knowing another area is close to its ceiling, and the
number is a fact about that area's work.
*/
func (s *Server) ListBudgetAlerts(ctx context.Context, req openapi.ListBudgetAlertsRequestObject) (openapi.ListBudgetAlertsResponseObject, error) {
	if s.marks == nil {
		return openapi.ListBudgetAlerts200JSONResponse{Items: []openapi.BudgetAlert{}}, nil
	}

	filter := runFilter(req.Params.Company, req.Params.Area, nil, nil, nil)
	filter, allowed := narrow(ctx, filter, domain.PermCostRead)
	if !allowed {
		return openapi.ListBudgetAlerts403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(
				domain.PermCostRead, scopeParams(req.Params.Company, req.Params.Area)),
		}, nil
	}

	announced, err := s.marks.Announced(ctx)
	if err != nil {
		return nil, fmt.Errorf("list budget alerts: %w", err)
	}

	visible := auth.VisibleScopes(ctx, domain.PermCostRead)
	items := make([]openapi.BudgetAlert, 0, len(announced))
	for _, mark := range announced {
		if !readable(mark.Scope, visible) {
			continue
		}
		if filter.Scope.Company != "" && !filter.Scope.Contains(mark.Scope) {
			continue
		}
		items = append(items, openapi.BudgetAlert{
			Scope: openapi.Scope{
				Company: string(mark.Scope.Company), Area: string(mark.Scope.Area),
			},
			Threshold:     mark.Threshold,
			SpentMicros:   mark.SpentMicros,
			CeilingMicros: mark.CeilingMicros,
			Since:         mark.Since,
		})
	}
	return openapi.ListBudgetAlerts200JSONResponse{Items: items}, nil
}
