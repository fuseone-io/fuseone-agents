package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// ListApprovals reads the inbox from the projection.
//
// The suspended action is denormalised onto the run row for exactly this: an
// approver opening their queue should cost one indexed read, not a fold of
// every run that ever waited on somebody.
func (s *Server) ListApprovals(ctx context.Context, req openapi.ListApprovalsRequestObject) (openapi.ListApprovalsResponseObject, error) {
	waiting, err := s.store.ListRuns(ctx,
		runFilter(req.Params.Company, req.Params.Area, nil, nil, nil),
		string(openapi.PhaseAwaitingApproval), limitOf(req.Params.Limit))
	if err != nil {
		return nil, fmt.Errorf("list approvals: %w", err)
	}

	page := openapi.ApprovalPage{Items: []openapi.PendingApproval{}}
	for _, run := range waiting {
		if run.PendingApproval == nil {
			continue
		}
		item := openapi.PendingApproval{
			RunId:   string(run.RunID),
			Scope:   &openapi.Scope{Company: string(run.Scope.Company), Area: string(run.Scope.Area)},
			AgentId: ptr(string(run.AgentID)),
			Tool:    string(run.PendingApproval.Tool),
			Rule:    ptr(run.PendingApproval.Rule),
			Reason:  ptr(run.PendingApproval.Reason),
			AtSeq:   run.PendingApproval.AtSeq,
			// When the run last moved. For a suspended run that is precisely
			// the moment it stopped to ask, and reading the step back to learn
			// it would undo the point of this query.
			RequestedAt: run.UpdatedAt,
		}
		page.Items = append(page.Items, item)
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
			Note:     valueOr(req.Body.Note),
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
