package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// rehearsalAbandonReason is what an abandoned rehearsal records as its reason: fixed,
// because an immutable row holds no free text.
const rehearsalAbandonReason = "rehearsal ended by its author"

/*
DecideInRehearsal records the author's decision where the Gate held an action.

It is rehearsal_decided, never approval_decided: it answers the Gate's request as a
person's would, and no inbox, count or audit of real approvals reads it. decideApproval
keeps refusing every simulated run, so a rehearsal has this door and no other.
*/
func (s *Server) DecideInRehearsal(
	ctx context.Context, req openapi.DecideInRehearsalRequestObject,
) (openapi.DecideInRehearsalResponseObject, error) {
	runID := domain.RunID(req.RunId)
	live, why, err := s.liveRehearsal(ctx, runID)
	switch {
	case err != nil:
		return nil, err
	case why == rehearsalAbsent:
		return openapi.DecideInRehearsal404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId)}, nil
	case why == notARehearsal:
		return openapi.DecideInRehearsal409ApplicationProblemPlusJSONResponse(conflicted("this run is not a live rehearsal")), nil
	}
	pending := live.state.PendingApproval
	if pending == nil || pending.AtSeq != req.Body.AtSeq {
		return openapi.DecideInRehearsal409ApplicationProblemPlusJSONResponse(conflicted(
			fmt.Sprintf("this rehearsal is not waiting for a decision at step %d", req.Body.AtSeq))), nil
	}
	decision := s.rehearsalDecision(ctx, live, *req.Body)
	head := domain.StepRef{Seq: req.Body.AtSeq, Kind: domain.StepApprovalRequested}
	switch _, err := s.store.AppendIfHead(ctx, head, decision); {
	case errors.Is(err, domain.ErrHeadMoved):
		return openapi.DecideInRehearsal409ApplicationProblemPlusJSONResponse(
			conflicted("this request was decided already, or the rehearsal moved on")), nil
	case err != nil:
		return nil, fmt.Errorf("record the decision for %s: %w", runID, err)
	}
	run, _, err := s.project(ctx, runID)
	if err != nil {
		return nil, err
	}
	return openapi.DecideInRehearsal200JSONResponse(run), nil
}

// rehearsalDecision is the step that records the author's decision on the held request.
func (s *Server) rehearsalDecision(ctx context.Context, live liveRun, body openapi.RehearsalDecision) domain.Step {
	state := live.state
	return domain.Step{
		RunID: state.RunID, Scope: state.Scope, AgentID: state.AgentID, VersionID: state.VersionID,
		OnBehalfOf: state.OnBehalfOf, Kind: domain.StepRehearsalDecided,
		At:      decidedAt(live.steps, clockOr(s.clock).Now()),
		IdemKey: fmt.Sprintf("rehearsal-decision:%s:%d", state.RunID, body.AtSeq),
		Payload: mustJSON(domain.RehearsalDecidedPayload{
			AtSeq: body.AtSeq, Approved: body.Approved, By: callerOf(ctx),
		}),
	}
}

/*
AbandonRehearsal ends a live rehearsal wherever it stands.

Nothing is undone, since nothing reached the world, so the run ends failed rather than
compensating. What it still holds reserved is released first, onto the head the request
was judged against, and the ending is sealed onto that release: a rehearsal never ends
holding a reservation, and one that moved in between is told so and writes nothing more.
A release whose ending then lost is harmless: the rehearsal goes on with nothing reserved,
and the author asks again.
*/
func (s *Server) AbandonRehearsal(
	ctx context.Context, req openapi.AbandonRehearsalRequestObject,
) (openapi.AbandonRehearsalResponseObject, error) {
	runID := domain.RunID(req.RunId)
	live, why, err := s.liveRehearsal(ctx, runID)
	switch {
	case err != nil:
		return nil, err
	case why == rehearsalAbsent:
		return openapi.AbandonRehearsal404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId)}, nil
	case why == notARehearsal:
		return openapi.AbandonRehearsal409ApplicationProblemPlusJSONResponse(conflicted("this run is not a live rehearsal")), nil
	case live.state.Terminal() || live.state.Phase == engine.PhaseCompensating:
		return openapi.AbandonRehearsal409ApplicationProblemPlusJSONResponse(conflicted(
			fmt.Sprintf("this rehearsal is %s", live.state.Phase))), nil
	}
	switch err := s.endRehearsal(ctx, live); {
	case errors.Is(err, domain.ErrHeadMoved):
		return openapi.AbandonRehearsal409ApplicationProblemPlusJSONResponse(
			conflicted("the rehearsal moved while it was being ended; ask again")), nil
	case err != nil:
		return nil, fmt.Errorf("end the rehearsal %s: %w", runID, err)
	}
	run, _, err := s.project(ctx, runID)
	if err != nil {
		return nil, err
	}
	return openapi.AbandonRehearsal200JSONResponse(run), nil
}

// endRehearsal releases what the rehearsal holds reserved, then seals its ending.
func (s *Server) endRehearsal(ctx context.Context, live liveRun) error {
	state, head := live.state, live.head()
	at := decidedAt(live.steps, clockOr(s.clock).Now())
	step := func(kind domain.StepKind, payload any) domain.Step {
		return domain.Step{
			RunID: state.RunID, Scope: state.Scope, AgentID: state.AgentID, VersionID: state.VersionID,
			OnBehalfOf: state.OnBehalfOf, Kind: kind, At: at, Payload: mustJSON(payload),
		}
	}
	ref := domain.StepRef{Seq: head.Seq, Kind: head.Kind}
	if state.Reserved.Micros != 0 || state.Reserved.Tokens != 0 {
		released, err := s.store.AppendIfHead(ctx, ref, step(domain.StepBudgetReconciled, domain.BudgetReconciledPayload{
			ReleasedMicros: state.Reserved.Micros, ReleasedTokens: state.Reserved.Tokens,
		}))
		if err != nil {
			return err
		}
		ref = domain.StepRef{Seq: released.Seq, Kind: released.Kind}
	}
	_, err := s.store.AppendIfHead(ctx, ref, step(domain.StepAbandoned, domain.AbandonedPayload{
		By: callerOf(ctx), Reason: rehearsalAbandonReason, Compensate: false,
	}))
	return err
}
