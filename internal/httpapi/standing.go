package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/standing"
)

// StandingApprovals is the store boundary, declared by the consumer.
type StandingApprovals interface {
	Create(ctx context.Context, g standing.Grant) error
	Revoke(ctx context.Context, id string, by domain.UserID, at time.Time) error
	List(ctx context.Context) ([]standing.Grant, error)
}

// StandingEvents records who granted and who revoked, beside the row itself.
type StandingEvents interface {
	RecordStanding(ctx context.Context, action, target string, by domain.UserID, scope domain.Scope) error
}

func (s *Server) WithStandingApprovals(store StandingApprovals, events StandingEvents) *Server {
	s.standing, s.standingEvents = store, events
	return s
}

func (s *Server) ListStandingApprovals(
	ctx context.Context, _ openapi.ListStandingApprovalsRequestObject,
) (openapi.ListStandingApprovalsResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermApprovalGrant); resp != nil {
		return openapi.ListStandingApprovals403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.standing == nil {
		return openapi.ListStandingApprovals200JSONResponse{Items: []openapi.StandingApproval{}}, nil
	}
	grants, err := s.standing.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list standing approvals: %w", err)
	}
	items := make([]openapi.StandingApproval, 0, len(grants))
	for _, g := range grants {
		items = append(items, standingApprovalFrom(g))
	}
	return openapi.ListStandingApprovals200JSONResponse{Items: items}, nil
}

/*
CreateStandingApproval grants ahead of time, which is strictly more than
clicking one card: the permission is its own, and the grant's invariants —
one exact tool, a ceiling, an expiry of at most 90 days, a reason — are
enforced by the store, not restated here.
*/
func (s *Server) CreateStandingApproval(
	ctx context.Context, req openapi.CreateStandingApprovalRequestObject,
) (openapi.CreateStandingApprovalResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermApprovalGrant); resp != nil {
		return openapi.CreateStandingApproval403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.standing == nil || req.Body == nil {
		return openapi.CreateStandingApproval400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("standing approvals are not configured")),
		}, nil
	}
	now := clockOr(s.clock).Now()
	expires := now.Add(standing.DefaultTTLDays * 24 * time.Hour)
	if req.Body.ExpiresAt != nil {
		expires = *req.Body.ExpiresAt
	}
	grant := standing.Grant{
		// Named after the instant it was decided, like a run is named after
		// the intention that opened it.
		ID:   "SG-" + strconv.FormatInt(now.UnixMilli(), 36),
		Tool: domain.ToolID(req.Body.ToolId), Agent: domain.AgentID(req.Body.AgentId),
		Scope: domain.Scope{
			Company: domain.CompanyID(req.Body.Scope.Company),
			Area:    domain.AreaID(req.Body.Scope.Area),
		},
		DailyCap: req.Body.DailyCap, Reason: req.Body.Reason,
		CreatedBy: callerOf(ctx), CreatedAt: now, ExpiresAt: expires,
	}
	if err := s.standing.Create(ctx, grant); err != nil {
		return openapi.CreateStandingApproval400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	s.recordStanding(ctx, "standing_approval.created", grant.ID, grant.CreatedBy, grant.Scope)
	return openapi.CreateStandingApproval204Response{}, nil
}

func (s *Server) RevokeStandingApproval(
	ctx context.Context, req openapi.RevokeStandingApprovalRequestObject,
) (openapi.RevokeStandingApprovalResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermApprovalGrant); resp != nil {
		return openapi.RevokeStandingApproval403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.standing == nil {
		return openapi.RevokeStandingApproval404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound("standing approvals are not configured"),
		}, nil
	}
	by := callerOf(ctx)
	switch err := s.standing.Revoke(ctx, req.Grant, by, clockOr(s.clock).Now()); {
	case errors.Is(err, standing.ErrGrantNotFound):
		return openapi.RevokeStandingApproval404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound("no active grant with that id"),
		}, nil
	case err != nil:
		return nil, fmt.Errorf("revoke standing approval: %w", err)
	}
	s.recordStanding(ctx, "standing_approval.revoked", req.Grant, by, domain.Scope{})
	return openapi.RevokeStandingApproval204Response{}, nil
}

// recordStanding writes the operator trail. The grant row itself carries who
// and when, so a failed event loses a convenience, not the record: logged
// nowhere rather than failing the operation that already happened.
func (s *Server) recordStanding(
	ctx context.Context, action, target string, by domain.UserID, scope domain.Scope,
) {
	if s.standingEvents == nil {
		return
	}
	_ = s.standingEvents.RecordStanding(ctx, action, target, by, scope)
}

func standingApprovalFrom(g standing.Grant) openapi.StandingApproval {
	out := openapi.StandingApproval{
		Id: g.ID, ToolId: string(g.Tool), AgentId: string(g.Agent),
		Scope:    openapi.Scope{Company: string(g.Scope.Company), Area: string(g.Scope.Area)},
		DailyCap: g.DailyCap, UsesToday: g.UsesToday,
		Reason: g.Reason, Status: openapi.StandingApprovalStatus(g.Status),
		CreatedBy: string(g.CreatedBy), CreatedAt: g.CreatedAt, ExpiresAt: g.ExpiresAt,
	}
	if g.RevokedBy != "" {
		out.RevokedBy = ptr(string(g.RevokedBy))
	}
	if g.RevokedAt != nil {
		out.RevokedAt = g.RevokedAt
	}
	return out
}
