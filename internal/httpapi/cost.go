package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

func (s *Server) GetCostRollup(ctx context.Context, req openapi.GetCostRollupRequestObject) (openapi.GetCostRollupResponseObject, error) {
	ids, err := s.store.Runs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	buckets := map[string]*openapi.CostBucket{}
	var total domain.Cost

	for _, id := range ids {
		steps, err := s.store.Read(ctx, id, domain.FirstSeq)
		if err != nil {
			return nil, err
		}
		if len(steps) == 0 || steps[0].At.Before(req.Params.From) || steps[0].At.After(req.Params.To) {
			continue
		}
		if !inScope(steps[0].Scope, req.Params.Company, req.Params.Area) {
			continue
		}

		var runCost domain.Cost
		for _, st := range steps {
			runCost = runCost.Add(st.Cost)
		}
		total = total.Add(runCost)

		key := bucketKey(steps[0], req.Params.GroupBy)
		b, ok := buckets[key]
		if !ok {
			b = &openapi.CostBucket{Key: key}
			buckets[key] = b
		}
		b.Cost = openapi.Cost{
			Micros:      b.Cost.Micros + runCost.Micros,
			InputTokens: ptr(deref(b.Cost.InputTokens) + runCost.TotalTokens()),
		}
		b.Runs++
	}

	out := openapi.CostRollup{
		From: req.Params.From, To: req.Params.To,
		GroupBy: groupByString(req.Params.GroupBy),
		Total:   toCost(total),
		Buckets: make([]openapi.CostBucket, 0, len(buckets)),
	}
	for _, b := range buckets {
		out.Buckets = append(out.Buckets, *b)
	}
	return openapi.GetCostRollup200JSONResponse(out), nil
}
