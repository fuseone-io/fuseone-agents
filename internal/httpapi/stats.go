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
	filter, allowed := narrow(ctx, runFilterFrom(req.Params), domain.PermRunRead)
	if !allowed {
		return openapi.GetRunStats403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermRunRead,
				scopeParams(req.Params.Company, req.Params.Area)),
		}, nil
	}

	stats, err := s.store.Stats(ctx, filter)
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
		median, p95 := stats.MedianDurationMS, stats.P95DurationMS
		body.MedianDurationMs, body.P95DurationMs = &median, &p95
	}

	return openapi.GetRunStats200JSONResponse(body), nil
}

// GetThroughput answers with runs per hour, split by what became of them.
//
// Same aggregation, same scoping, different shape: the overview asks how the
// day went rather than how it ended, and a single tally cannot say that.
func (s *Server) GetThroughput(
	ctx context.Context, req openapi.GetThroughputRequestObject,
) (openapi.GetThroughputResponseObject, error) {
	filter, allowed := narrow(ctx, throughputFilterFrom(req.Params), domain.PermRunRead)
	if !allowed {
		return openapi.GetThroughput403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermRunRead,
				scopeParams(req.Params.Company, req.Params.Area)),
		}, nil
	}

	buckets, err := s.store.Throughput(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("throughput: %w", err)
	}

	out := openapi.Throughput{Buckets: make([]openapi.ThroughputBucket, 0, len(buckets))}
	for _, b := range buckets {
		out.Buckets = append(out.Buckets, openapi.ThroughputBucket{
			At: b.At, Total: b.Total, Micros: b.Micros,
			ByPhase: b.ByPhase, ByAgent: b.ByAgent,
		})
	}
	return openapi.GetThroughput200JSONResponse(out), nil
}

func throughputFilterFrom(p openapi.GetThroughputParams) domain.RunFilter {
	return runFilterFrom(openapi.GetRunStatsParams{
		Company: p.Company, Area: p.Area, AgentId: p.AgentId,
		Since: p.Since, Until: p.Until,
	})
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
	if p.Until != nil {
		f.Until = *p.Until
	}
	return f
}

// ListDecisions answers with what the Gate decided across runs.
//
// Read across the ledger rather than down one run: it is how somebody sees
// whether the installation's rules are engaging at all. Scoped like every
// other read, because a decision names a tool and an agent, and that is
// information about the area they belong to.
func (s *Server) ListDecisions(
	ctx context.Context, req openapi.ListDecisionsRequestObject,
) (openapi.ListDecisionsResponseObject, error) {
	filter, allowed := narrow(ctx, decisionFilterFrom(req.Params), domain.PermRunRead)
	if !allowed {
		return openapi.ListDecisions403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermRunRead,
				scopeParams(req.Params.Company, req.Params.Area)),
		}, nil
	}

	decisions, err := s.store.Decisions(ctx, filter, limitOf(req.Params.Limit))
	if err != nil {
		return nil, fmt.Errorf("decisions: %w", err)
	}

	page := openapi.DecisionPage{Items: make([]openapi.RecordedDecision, 0, len(decisions))}
	for _, d := range decisions {
		page.Items = append(page.Items, openapi.RecordedDecision{
			RunId:   string(d.RunID),
			Seq:     d.Seq,
			At:      d.At,
			Scope:   &openapi.Scope{Company: string(d.Scope.Company), Area: string(d.Scope.Area)},
			AgentId: ptr(string(d.AgentID)),
			Tool:    ptr(string(d.Tool)),
			Verdict: openapi.Verdict(d.Verdict.String()),
			Rule:    ptr(d.Rule),
		})
	}
	return openapi.ListDecisions200JSONResponse(page), nil
}

func decisionFilterFrom(p openapi.ListDecisionsParams) domain.RunFilter {
	return runFilterFrom(openapi.GetRunStatsParams{
		Company: p.Company, Area: p.Area, AgentId: p.AgentId, Since: p.Since,
	})
}
