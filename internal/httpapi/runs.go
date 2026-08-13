// Package httpapi implements the OpenAPI contract in api/openapi.yaml.
//
// The interface it satisfies is generated from that file, so an endpoint that
// drifts from the contract fails to compile rather than failing in a customer
// integration.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Reading a run: the list, the run itself, its trail, and the re-check.

All four are folds of the ledger rather than stored rows, and none of them can
show a step to somebody whose labels do not reach it — the filter is here and
not in the query, so a new reader cannot forget it (PRD AU-05).
*/
// ListRuns reads a page from the projection.
//
// It used to read every run and fold it until enough matched the filter, which
// made a single page view cost the whole ledger. The projection exists for
// exactly this.
func (s *Server) ListRuns(ctx context.Context, req openapi.ListRunsRequestObject) (openapi.ListRunsResponseObject, error) {
	var phase string
	if req.Params.Phase != nil {
		phase = string(*req.Params.Phase)
	}

	filter := runFilter(req.Params.Company, req.Params.Area, req.Params.AgentId, req.Params.Since, nil)
	if req.Params.Q != nil {
		filter.Search = strings.TrimSpace(*req.Params.Q)
	}

	// Every read of a run is scoped. This endpoint predates authentication and
	// was answering with every run in the installation to anybody with a
	// session (PRD NF-06).
	filter, allowed := narrow(ctx, filter, domain.PermRunRead)
	if !allowed {
		return forbiddenListRuns(domain.PermRunRead, scopeParams(req.Params.Company, req.Params.Area)), nil
	}

	summaries, err := s.store.ListRuns(ctx, filter, phase, limitOf(req.Params.Limit))
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	items := make([]openapi.Run, 0, len(summaries))
	for _, summary := range summaries {
		items = append(items, runFromSummary(summary))
	}
	return openapi.ListRuns200JSONResponse{Items: items}, nil
}

func (s *Server) GetRun(ctx context.Context, req openapi.GetRunRequestObject) (openapi.GetRunResponseObject, error) {
	run, state, err := s.project(ctx, domain.RunID(req.RunId))
	if err != nil {
		if isNotFound(err) {
			return openapi.GetRun404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
			}, nil
		}
		return nil, err
	}
	// Not found rather than forbidden: telling somebody a run exists in an
	// area they cannot see is itself information about that area.
	if !mayRead(ctx, domain.PermRunRead, state.Scope) {
		return openapi.GetRun404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
		}, nil
	}
	return openapi.GetRun200JSONResponse(run), nil
}

func (s *Server) ListRunSteps(ctx context.Context, req openapi.ListRunStepsRequestObject) (openapi.ListRunStepsResponseObject, error) {
	from := int64(domain.FirstSeq)
	if req.Params.FromSeq != nil {
		from = *req.Params.FromSeq
	}

	// The trail carries tool names, rules, reasons and labels. Reading another
	// area's is reading what its agents were asked to do.
	if !s.readableRun(ctx, domain.RunID(req.RunId)) {
		return openapi.ListRunSteps404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
		}, nil
	}

	steps, err := s.store.Read(ctx, domain.RunID(req.RunId), from)
	if err != nil {
		if isNotFound(err) {
			return openapi.ListRunSteps404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
			}, nil
		}
		return nil, fmt.Errorf("read steps: %w", err)
	}

	limit := limitOf(req.Params.Limit)
	page := openapi.StepPage{Items: make([]openapi.Step, 0, min(limit, len(steps)))}
	for _, st := range steps {
		if len(page.Items) == limit {
			page.NextSeq = ptr(st.Seq)
			break
		}
		page.Items = append(page.Items, toStep(st))
	}
	return openapi.ListRunSteps200JSONResponse(page), nil
}

func (s *Server) VerifyRun(ctx context.Context, req openapi.VerifyRunRequestObject) (openapi.VerifyRunResponseObject, error) {
	steps, err := s.store.Read(ctx, domain.RunID(req.RunId), domain.FirstSeq)
	if err != nil {
		if isNotFound(err) {
			return openapi.VerifyRun404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
			}, nil
		}
		return nil, fmt.Errorf("read steps: %w", err)
	}
	if len(steps) == 0 || !mayRead(ctx, domain.PermRunRead, steps[0].Scope) {
		return openapi.VerifyRun404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
		}, nil
	}

	result := openapi.VerifyResult{Valid: true, StepsChecked: int64(len(steps))}
	if err := domain.VerifyChain(steps); err != nil {
		result.Valid = false
		result.StepsChecked = int64(len(steps))
		result.BrokenAtSeq = ptr(firstBrokenSeq(steps))
	}
	return openapi.VerifyRun200JSONResponse(result), nil
}

// project folds a run and shapes it for the wire.
//
// Folding on every read is honest for a development ledger and will not
// survive real volume; Postgres will serve this from a materialised runs
// projection maintained on append.
func (s *Server) project(ctx context.Context, id domain.RunID) (openapi.Run, engine.State, error) {
	steps, err := s.store.Read(ctx, id, domain.FirstSeq)
	if err != nil {
		return openapi.Run{}, engine.State{}, err
	}
	state, err := engine.Fold(steps)
	if err != nil {
		return openapi.Run{}, engine.State{}, fmt.Errorf("fold %s: %w", id, err)
	}
	return toRun(id, state, steps), state, nil
}

func firstBrokenSeq(steps []domain.Step) int64 {
	var prev *domain.Step
	for i := range steps {
		if err := steps[i].VerifyLink(prev); err != nil {
			return steps[i].Seq
		}
		prev = &steps[i]
	}
	return 0
}

func limitOf(v *int) int {
	if v == nil || *v <= 0 {
		return defaultLimit
	}
	return *v
}

func isNotFound(err error) bool {
	return err != nil && errors.Is(err, errNotFound) ||
		(err != nil && err.Error() == "ledger: run not found")
}

var errNotFound = errors.New("not found")

// readableRun reports whether the caller may read a run at all.
//
// A run that does not exist and a run in somebody else's area answer the same
// way on purpose: confirming that a run exists is information about the area
// it belongs to.
func (s *Server) readableRun(ctx context.Context, runID domain.RunID) bool {
	steps, err := s.store.Read(ctx, runID, domain.FirstSeq)
	if err != nil || len(steps) == 0 {
		return false
	}
	return mayRead(ctx, domain.PermRunRead, steps[0].Scope)
}
