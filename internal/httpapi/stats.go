package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// GetRunStats answers with counts over every matching run.
//
// The figures the console prints as facts are computed where the whole set is.
// Deriving them from a page would make "97% concluded" mean "97% of the fifty
// runs that happened to load", which is not what any reader would take it for.
func (s *Server) GetRunStats(ctx context.Context, req openapi.GetRunStatsRequestObject) (openapi.GetRunStatsResponseObject, error) {
	stats, err := s.store.Stats(ctx, runFilterFrom(req.Params))
	if err != nil {
		return nil, fmt.Errorf("run stats: %w", err)
	}

	body := openapi.RunStats{
		Total:   stats.Total,
		Ended:   stats.Ended,
		ByPhase: stats.ByPhase,
	}
	// Absent rather than zero when nothing has ended: a median of "0ms" reads
	// as a measurement, and there is nothing to measure yet.
	if stats.Ended > 0 {
		median := stats.MedianDurationMS
		body.MedianDurationMs = &median
	}

	return openapi.GetRunStats200JSONResponse(body), nil
}

func runFilterFrom(p openapi.GetRunStatsParams) domain.RunFilter {
	var f domain.RunFilter
	if p.Company != nil {
		f.Scope.Company = domain.CompanyID(*p.Company)
	}
	if p.Area != nil {
		f.Scope.Area = domain.AreaID(*p.Area)
	}
	if p.AgentId != nil {
		f.AgentID = domain.AgentID(*p.AgentId)
	}
	if p.Since != nil {
		f.Since = *p.Since
	}
	return f
}
