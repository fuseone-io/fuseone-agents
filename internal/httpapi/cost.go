package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// GetCostRollup sums where the whole set is.
//
// It used to read every step of every run to add money up, which made the cost
// page the most expensive thing an installation did — and it reported the token
// breakdown as one lump, losing the split the contract promises.
func (s *Server) GetCostRollup(ctx context.Context, req openapi.GetCostRollupRequestObject) (openapi.GetCostRollupResponseObject, error) {
	groupBy := groupByString(req.Params.GroupBy)

	// Spend is scoped like everything else: what an area costs is that area's
	// business, and the monthly close is where somebody would learn it.
	filter, allowed := narrow(ctx, runFilter(
		req.Params.Company, req.Params.Area, nil, &req.Params.From, &req.Params.To),
		domain.PermCostRead)
	if !allowed {
		return openapi.GetCostRollup403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermCostRead,
				scopeParams(req.Params.Company, req.Params.Area)),
		}, nil
	}

	// The contract constrains groupBy to a closed enum and requires the window,
	// so a refusal from the store here is a bug rather than a bad request.
	buckets, err := s.store.CostRollup(ctx, filter, groupBy)
	if err != nil {
		return nil, fmt.Errorf("cost rollup: %w", err)
	}

	out := openapi.CostRollup{
		From: req.Params.From, To: req.Params.To, GroupBy: groupBy,
		Buckets: make([]openapi.CostBucket, 0, len(buckets)),
	}

	var total domain.Cost
	for _, b := range buckets {
		total = total.Add(b.Cost)
		out.Buckets = append(out.Buckets, openapi.CostBucket{
			Key: b.Key, Runs: b.Runs, Cost: toCost(b.Cost),
		})
	}
	out.Total = toCost(total)

	return openapi.GetCostRollup200JSONResponse(out), nil
}
