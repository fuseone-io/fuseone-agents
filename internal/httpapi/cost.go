package httpapi

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/finops"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// PlanningSpend is the read model for planning-call spend, declared here by
// the consumer. It is separate from Store because it is a sweep-owned
// projection, not a ledger fold.
type PlanningSpend interface {
	ByModel(ctx context.Context, filter domain.RunFilter) ([]finops.Bucket, error)
	ByAgent(ctx context.Context, filter domain.RunFilter) ([]finops.Bucket, error)
	ProjectedFrom(ctx context.Context) (time.Time, error)
}

// WithPlanningSpend wires the forward-only planning spend projection.
func (s *Server) WithPlanningSpend(spend PlanningSpend) *Server {
	s.planning = spend
	return s
}

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

// GetPlanningSpendByModel explains which effective models spent the planning
// budget. It reads a projection that starts when the worker learned to record
// the effective provider/model pair, and says so in the response instead of
// backfilling a guess.
func (s *Server) GetPlanningSpendByModel(
	ctx context.Context, req openapi.GetPlanningSpendByModelRequestObject,
) (openapi.GetPlanningSpendByModelResponseObject, error) {
	filter, allowed := narrow(ctx, runFilter(
		req.Params.Company, req.Params.Area, nil, &req.Params.From, &req.Params.To),
		domain.PermCostRead)
	if !allowed {
		return openapi.GetPlanningSpendByModel403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermCostRead,
				scopeParams(req.Params.Company, req.Params.Area)),
		}, nil
	}

	out, err := s.planningSpendRollup(ctx, req.Params.From, req.Params.To,
		openapi.PlanningSpendRollupGroupByModel, func() ([]finops.Bucket, error) {
			if s.planning == nil {
				return nil, nil
			}
			return s.planning.ByModel(ctx, filter)
		})
	if err != nil {
		return nil, fmt.Errorf("planning spend by model: %w", err)
	}
	return openapi.GetPlanningSpendByModel200JSONResponse(out), nil
}

// GetPlanningSpendByAgent explains which agents spent planning budget. The
// bucket deliberately carries no provider/model: an agent bucket may fold
// several, and naming one would make an arbitrary row look authoritative.
func (s *Server) GetPlanningSpendByAgent(
	ctx context.Context, req openapi.GetPlanningSpendByAgentRequestObject,
) (openapi.GetPlanningSpendByAgentResponseObject, error) {
	filter, allowed := narrow(ctx, runFilter(
		req.Params.Company, req.Params.Area, nil, &req.Params.From, &req.Params.To),
		domain.PermCostRead)
	if !allowed {
		return openapi.GetPlanningSpendByAgent403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermCostRead,
				scopeParams(req.Params.Company, req.Params.Area)),
		}, nil
	}

	out, err := s.planningSpendRollup(ctx, req.Params.From, req.Params.To,
		openapi.PlanningSpendRollupGroupByAgent, func() ([]finops.Bucket, error) {
			if s.planning == nil {
				return nil, nil
			}
			return s.planning.ByAgent(ctx, filter)
		})
	if err != nil {
		return nil, fmt.Errorf("planning spend by agent: %w", err)
	}
	return openapi.GetPlanningSpendByAgent200JSONResponse(out), nil
}

func (s *Server) planningSpendRollup(
	ctx context.Context, from, to time.Time, groupBy openapi.PlanningSpendRollupGroupBy,
	read func() ([]finops.Bucket, error),
) (openapi.PlanningSpendRollup, error) {
	projectedFrom, err := s.planningProjectedFrom(ctx)
	if err != nil {
		return openapi.PlanningSpendRollup{}, err
	}

	buckets, err := read()
	if err != nil {
		return openapi.PlanningSpendRollup{}, err
	}
	out := openapi.PlanningSpendRollup{
		From: from, To: to, ProjectedFrom: projectedFrom, GroupBy: groupBy,
		Buckets: make([]openapi.PlanningSpendBucket, 0, len(buckets)),
	}
	var total domain.Cost
	for _, b := range buckets {
		bucket, cost := planningSpendBucket(b)
		total = total.Add(cost)
		out.Calls += b.Calls
		out.Unpriced += b.Unpriced
		out.Buckets = append(out.Buckets, bucket)
	}
	out.Total = toCost(total)
	return out, nil
}

func (s *Server) planningProjectedFrom(ctx context.Context) (*time.Time, error) {
	if s.planning == nil {
		return nil, nil
	}
	at, err := s.planning.ProjectedFrom(ctx)
	if err != nil {
		return nil, err
	}
	if at.IsZero() {
		return nil, nil
	}
	return ptr(at), nil
}

func planningSpendBucket(b finops.Bucket) (openapi.PlanningSpendBucket, domain.Cost) {
	cost := domain.Cost{
		Micros:           b.Micros,
		InputTokens:      b.InputTokens,
		OutputTokens:     b.OutputTokens,
		CacheReadTokens:  b.CacheReadTokens,
		CacheWriteTokens: b.CacheWriteTokens,
	}
	return openapi.PlanningSpendBucket{
		Provider: b.Provider, Model: b.Model, Agent: b.Agent,
		Calls: b.Calls, Runs: b.Runs, Cost: toCost(cost), Unpriced: b.Unpriced,
	}, cost
}
