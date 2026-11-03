package httpapi

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
GET /work-items/measures: the ledger's weekly measures of the factory (#201,
PR 1) — per ISO week in UTC and agent, the runs, the cards, what they asked
people and how people answered, and where the cost went.

Access is the trail's, derived from the caller's own grants: runs where the
caller holds run:read and agent:read, cost where they also hold cost:read
over every scope a row folds. The window is the server's clock's, so a test
states it and a caller cannot move it. What the ledger does not record is
said once, on every response, and never as a zero.
*/

func (s *Server) GetWorkItemMeasures(
	ctx context.Context, req openapi.GetWorkItemMeasuresRequestObject,
) (openapi.GetWorkItemMeasuresResponseObject, error) {
	if len(s.grantsFor()(ctx, domain.PermRunRead)) == 0 {
		return openapi.GetWorkItemMeasures403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermRunRead, domain.Scope{}),
		}, nil
	}
	weeks := domain.DefaultMeasureWeeks
	if req.Params.Weeks != nil {
		weeks = *req.Params.Weeks
	}
	now := clockOr(s.clock).Now()
	window, err := domain.MeasureWindow(now, weeks)
	if err != nil { // domain.ErrMeasureWeeks, the only way it fails
		return openapi.GetWorkItemMeasures400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	filter := runFilter(req.Params.Company, req.Params.Area, req.Params.AgentId, nil, nil)
	rows, err := s.workItemMeasures(ctx, s.trailAccessOf(ctx), filter, window)
	if err != nil {
		return nil, err
	}
	return openapi.GetWorkItemMeasures200JSONResponse(toWorkItemMeasures(window, now, rows)), nil
}

// workItemMeasures reads the rows under an access already derived from the
// caller. It never asks the store with no scopes: to the store an empty
// filter is no narrowing at all.
func (s *Server) workItemMeasures(
	ctx context.Context, access trailAccess, filter domain.RunFilter, window domain.WorkItemWindow,
) ([]domain.WorkItemWeek, error) {
	if s.workItems == nil || len(access.runs) == 0 {
		return nil, nil
	}
	filter.Scopes = access.runs
	var cost *domain.RunFilter
	if len(access.cost) > 0 {
		cost = &domain.RunFilter{Scopes: access.cost}
	}
	rows, err := s.workItems.WorkItemMeasures(ctx, filter, cost, window)
	if err != nil {
		return nil, fmt.Errorf("read the work item measures: %w", err)
	}
	return rows, nil
}

func toWorkItemMeasures(window domain.WorkItemWindow, now time.Time, rows []domain.WorkItemWeek) openapi.WorkItemMeasures {
	out := openapi.WorkItemMeasures{
		WeekStart: openapi.Monday, Timezone: openapi.UTC,
		From: window.From, To: window.To, AsOf: now,
		Rows: make([]openapi.WorkItemMeasureRow, 0, len(rows)),
	}
	for _, r := range rows {
		out.Rows = append(out.Rows, toWorkItemMeasureRow(r, now))
	}
	for _, u := range domain.UnavailableWorkItemMeasures() {
		out.Unavailable = append(out.Unavailable, openapi.UnavailableMeasure{
			Measure: openapi.UnavailableMeasureMeasure(u.Measure),
			Reason:  openapi.UnavailableMeasureReason(u.Reason),
			Detail:  u.Detail,
		})
	}
	return out
}

func toWorkItemMeasureRow(r domain.WorkItemWeek, now time.Time) openapi.WorkItemMeasureRow {
	p := r.Proposals
	row := openapi.WorkItemMeasureRow{
		Week: r.Week, Complete: domain.WeekComplete(r.Week, now), AgentId: string(r.AgentID),
		Runs: r.Runs, OpenRuns: r.OpenRuns, Cards: r.Cards,
		Proposals: openapi.WorkItemProposals{
			Requested: p.Requested, Approved: p.Approved, Refused: p.Refused, Abandoned: p.Abandoned,
			Pending: p.Pending, RevisedAfterRefusal: p.RevisedAfterRefusal,
			RepeatedAfterRefusal: p.RepeatedAfterRefusal, Standing: p.Standing,
		},
	}
	if r.Cost != nil {
		row.CostOnCardsMicros = ptr(r.Cost.OnCardsMicros)
		row.CostInheritedMicros = ptr(r.Cost.InheritedMicros)
		row.CostOffCardMicros = ptr(r.Cost.OffCardMicros)
	}
	return row
}
