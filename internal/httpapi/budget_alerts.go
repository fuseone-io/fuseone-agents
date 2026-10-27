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

A recorded mark is served only while it still describes the budget as
configured. Marks are the sweeper's dedupe memory and are never erased, so
without this check a ceiling raised after a crossing keeps its exhausted
banner on the screen for ever (#146).
*/
func (s *Server) ListBudgetAlerts(ctx context.Context, req openapi.ListBudgetAlertsRequestObject) (openapi.ListBudgetAlertsResponseObject, error) {
	if s.marks == nil || s.ceilings == nil {
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
	configured, err := s.ceilings.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list budget alerts: read ceilings: %w", err)
	}
	ceilings := make(map[string]domain.ScopeBudget, len(configured))
	for _, c := range configured {
		ceilings[c.Scope.String()] = c
	}

	now := clockOr(s.clock).Now()
	visible := auth.VisibleScopes(ctx, domain.PermCostRead)
	items := make([]openapi.BudgetAlert, 0, len(announced))
	for _, mark := range announced {
		ceiling := ceilings[mark.Scope.String()]
		if !mark.Describes(ceiling, now) {
			continue
		}
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
			// From the ceiling rather than the mark: the mark never stored
			// it, and the match above makes the ceiling's word authoritative.
			Period: openapi.BudgetAlertPeriod(ceiling.Period),
		})
	}
	return openapi.ListBudgetAlerts200JSONResponse{Items: items}, nil
}
