package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

func (s *Server) ListApprovals(ctx context.Context, req openapi.ListApprovalsRequestObject) (openapi.ListApprovalsResponseObject, error) {
	ids, err := s.store.Runs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	page := openapi.ApprovalPage{Items: []openapi.PendingApproval{}}
	limit := limitOf(req.Params.Limit)

	for _, id := range ids {
		if len(page.Items) == limit {
			break
		}
		steps, err := s.store.Read(ctx, id, domain.FirstSeq)
		if err != nil {
			return nil, err
		}
		state, err := engine.Fold(steps)
		if err != nil {
			return nil, err
		}
		if state.PendingApproval == nil {
			continue
		}
		if !inScope(state.Scope, req.Params.Company, req.Params.Area) {
			continue
		}
		pa := toPendingApproval(id, state)
		pa.RequestedAt = steps[state.PendingApproval.AtSeq-1].At
		page.Items = append(page.Items, pa)
	}
	return openapi.ListApprovals200JSONResponse(page), nil
}

func (s *Server) DecideApproval(ctx context.Context, req openapi.DecideApprovalRequestObject) (openapi.DecideApprovalResponseObject, error) {
	runID := domain.RunID(req.RunId)

	steps, err := s.store.Read(ctx, runID, domain.FirstSeq)
	if err != nil {
		if isNotFound(err) {
			return openapi.DecideApproval404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
			}, nil
		}
		return nil, err
	}
	state, err := engine.Fold(steps)
	if err != nil {
		return nil, err
	}

	// atSeq is required by the contract so a stale tab cannot answer an
	// approval that a later step already superseded.
	switch {
	case state.PendingApproval == nil:
		return openapi.DecideApproval409ApplicationProblemPlusJSONResponse(
			problem(http.StatusConflict, "No pending approval", "This run is not awaiting a decision")), nil
	case state.PendingApproval.AtSeq != req.Body.AtSeq:
		return openapi.DecideApproval409ApplicationProblemPlusJSONResponse(problem(
			http.StatusConflict, "Stale approval",
			fmt.Sprintf("This run is awaiting a decision on step %d, not %d",
				state.PendingApproval.AtSeq, req.Body.AtSeq))), nil
	}

	last := steps[len(steps)-1]
	if _, err := s.store.Append(ctx, domain.Step{
		RunID: runID, Scope: state.Scope,
		AgentID: state.AgentID, VersionID: state.VersionID, OnBehalfOf: state.OnBehalfOf,
		Kind: domain.StepApprovalDecided,
		At:   last.At,
		Payload: mustJSON(domain.ApprovalDecidedPayload{
			Approved: req.Body.Approved,
			Note:     deref(req.Body.Note),
		}),
	}); err != nil {
		return nil, fmt.Errorf("record decision: %w", err)
	}

	run, _, err := s.project(ctx, runID)
	if err != nil {
		return nil, err
	}
	return openapi.DecideApproval200JSONResponse(run), nil
}
