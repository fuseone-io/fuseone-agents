package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
GET /work-items/trail: a card's runs and, per role, what they asked people
and what they cost (#204, slice 1).

Every page derives the caller's access from the caller's own grants — never
from the cursor, which is a position and nothing else. A run is read where
the caller holds both run:read and agent:read (the #230 rule: an agent id is
named only to somebody who may read the agent), cost where they also hold
cost:read. A reference with no readable run is an empty trail, the same
bytes as one nobody claimed: a 404 or a different shape would tell the
caller the card is worked on somewhere they cannot see.
*/

// WorkItems is the trail's store, declared by the consumer.
type WorkItems interface {
	WorkItemTrail(ctx context.Context, filter domain.RunFilter, cost *domain.RunFilter,
		ref domain.WorkItemRef, page domain.WorkItemPage) (domain.WorkItemTrail, error)
}

// WithWorkItems wires the trail's store. Without it every trail is empty.
func (s *Server) WithWorkItems(w WorkItems) *Server {
	s.workItems = w
	return s
}

// trailAccess is what one caller may read on a trail: runs where they hold
// run:read and agent:read, cost where they hold cost:read.
type trailAccess struct{ runs, cost []domain.Scope }

func (s *Server) trailAccessOf(ctx context.Context) trailAccess {
	grants := s.grantsFor()
	return trailAccess{
		runs: trailScopes(grants(ctx, domain.PermRunRead), grants(ctx, domain.PermAgentRead)),
		cost: grants(ctx, domain.PermCostRead),
	}
}

// grantsFor is where the caller holds a permission: the caller's own grants,
// as every other read asks. Only a test sets trailGrants, to state a caller
// no shipped role describes — run:read without agent:read or cost:read — so
// the rules for those combinations are proved through this handler rather
// than beside it.
func (s *Server) grantsFor() func(context.Context, domain.Permission) []domain.Scope {
	if s.trailGrants != nil {
		return s.trailGrants
	}
	return auth.VisibleScopes
}

// trailScopes is where the caller reads both runs and agents. Scopes nest
// (installation, company, area), so this is each scope of one list that a
// scope of the other reaches.
func trailScopes(runs, agents []domain.Scope) []domain.Scope {
	var out []domain.Scope
	for _, s := range runs {
		if readable(s, agents) {
			out = append(out, s)
		}
	}
	for _, a := range agents {
		if readable(a, runs) && !readable(a, out) {
			out = append(out, a)
		}
	}
	return out
}

func (s *Server) GetWorkItemTrail(
	ctx context.Context, req openapi.GetWorkItemTrailRequestObject,
) (openapi.GetWorkItemTrailResponseObject, error) {
	if len(s.grantsFor()(ctx, domain.PermRunRead)) == 0 {
		return openapi.GetWorkItemTrail403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermRunRead, domain.Scope{}),
		}, nil
	}
	ref, ok := domain.ParseWorkItemRef(req.Params.Ref)
	if !ok {
		return badTrail("ref is not a work item reference"), nil
	}
	page := domain.WorkItemPage{Limit: limitOf(req.Params.Limit)}
	if req.Params.Cursor != nil {
		after, err := domain.DecodeWorkItemCursor(*req.Params.Cursor)
		if err != nil {
			return badTrail("cursor is not a position on this trail"), nil
		}
		page.After = after
	}
	return s.workItemTrail(ctx, s.trailAccessOf(ctx), ref, page)
}

// workItemTrail reads one page under an access already derived from the
// caller. It never asks the store with no scopes: to the store an empty
// filter is no narrowing at all.
func (s *Server) workItemTrail(
	ctx context.Context, access trailAccess, ref domain.WorkItemRef, page domain.WorkItemPage,
) (openapi.GetWorkItemTrailResponseObject, error) {
	trail := domain.WorkItemTrail{Ref: ref}
	if page.After == nil {
		trail.Roles = []domain.WorkItemRole{}
	}
	if s.workItems != nil && len(access.runs) > 0 {
		var cost *domain.RunFilter
		if len(access.cost) > 0 {
			cost = &domain.RunFilter{Scopes: access.cost}
		}
		read, err := s.workItems.WorkItemTrail(ctx, domain.RunFilter{Scopes: access.runs}, cost, ref, page)
		if err != nil {
			return nil, fmt.Errorf("read the work item trail: %w", err)
		}
		trail = read
	}
	return openapi.GetWorkItemTrail200JSONResponse(toWorkItemTrail(trail)), nil
}

func badTrail(detail string) openapi.GetWorkItemTrail400ApplicationProblemPlusJSONResponse {
	return openapi.GetWorkItemTrail400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(detail)),
	}
}

func toWorkItemTrail(t domain.WorkItemTrail) openapi.WorkItemTrail {
	out := openapi.WorkItemTrail{Ref: string(t.Ref), Runs: make([]openapi.WorkItemRun, 0, len(t.Runs))}
	for _, r := range t.Runs {
		out.Runs = append(out.Runs, toWorkItemRun(r))
	}
	if t.Roles != nil {
		roles := make([]openapi.WorkItemRole, 0, len(t.Roles))
		for _, r := range t.Roles {
			roles = append(roles, toWorkItemRole(r))
		}
		out.Roles = &roles
	}
	if t.Next != nil {
		out.NextCursor = ptr(t.Next.Encode())
	}
	return out
}

func toWorkItemRun(r domain.WorkItemRun) openapi.WorkItemRun {
	run := openapi.WorkItemRun{
		RunId: string(r.RunID), AgentId: string(r.AgentID),
		Scope: openapi.Scope{Company: string(r.Scope.Company), Area: string(r.Scope.Area)},
		Phase: openapi.Phase(r.Phase), StartedAt: r.StartedAt, ClaimedAt: r.ClaimedAt,
		Link: openapi.WorkItemRunLink(r.Link),
	}
	if !r.EndedAt.IsZero() {
		run.EndedAt = ptr(r.EndedAt)
	}
	return run
}

func toWorkItemRole(r domain.WorkItemRole) openapi.WorkItemRole {
	role := openapi.WorkItemRole{
		AgentId: string(r.AgentID), Runs: r.Runs,
		ApprovalsRequested: r.Approvals.Requested, Approved: r.Approvals.Approved,
		Refused: r.Approvals.Refused, Abandoned: r.Approvals.Abandoned,
		FirstClaimedAt: r.FirstClaimedAt, CostMicros: r.CostMicros,
	}
	if !r.LastEndedAt.IsZero() {
		role.LastEndedAt = ptr(r.LastEndedAt)
	}
	return role
}
