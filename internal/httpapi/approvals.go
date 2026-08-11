package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
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

	// What the action does is part of the ask: an approver deciding on a
	// refund needs to know it is a refund. The effect is read here rather than
	// by the caller, who holds approval:act and deliberately not tool:read —
	// seeing one action's classification is not seeing the catalogue.
	effects := s.toolEffects(ctx)

	// The inbox shows what the caller may act on. Reading somebody else's
	// queue tells them which actions an agent proposed in an area they have
	// no part in (PRD NF-06).
	visible := auth.VisibleScopes(ctx, domain.PermApprovalAct)

	page := openapi.ApprovalPage{Items: []openapi.PendingApproval{}}
	for _, run := range waiting {
		if run.PendingApproval == nil || !readable(run.Scope, visible) {
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
		if effect, known := effects[run.PendingApproval.Tool]; known {
			item.Effect = ptr(openapi.Effect(effect.String()))
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

	// Deciding is the whole point of the approval gate, and it was reachable
	// by any authenticated caller. The check is against the run's own scope:
	// an approver in cx does not decide for marketing.
	if err := auth.Require(ctx, domain.PermApprovalAct, state.Scope); err != nil {
		return openapi.DecideApproval403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermApprovalAct, state.Scope),
		}, nil
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

// toolEffects reads the published catalogue, or nothing when this installation
// has no administration store. An unknown effect leaves the field absent
// rather than guessing "read", which would understate what is being asked for.
func (s *Server) toolEffects(ctx context.Context) map[domain.ToolID]domain.Effect {
	if s.tools == nil {
		return nil
	}
	entries, err := s.tools.Tools(ctx)
	if err != nil {
		// A catalogue that cannot be read must not stop an approver from
		// seeing their queue; the ask is still legible without the effect.
		return nil
	}

	out := make(map[domain.ToolID]domain.Effect, len(entries))
	for _, e := range entries {
		out[e.ID] = e.Effect
	}
	return out
}
