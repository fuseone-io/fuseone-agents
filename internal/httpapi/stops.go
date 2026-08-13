package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Stoppers are the switches that stop the platform without a deploy, declared
// here by the consumer (PRD FO-06).
type Stoppers interface {
	InForce(ctx context.Context) ([]domain.Stop, error)
	Stop(ctx context.Context, stop domain.Stop) error
	Start(ctx context.Context, stop domain.Stop) error
}

// ListStops is what is currently off. Readable by anyone who can read runs:
// "why is nothing running" is the question this answers, and making people ask
// an administrator for it is how an incident gets longer.
func (s *Server) ListStops(ctx context.Context, _ openapi.ListStopsRequestObject) (openapi.ListStopsResponseObject, error) {
	if s.stops == nil {
		return openapi.ListStops200JSONResponse{Items: []openapi.Stop{}}, nil
	}
	inForce, err := s.stops.InForce(ctx)
	if err != nil {
		return nil, fmt.Errorf("list stops: %w", err)
	}

	items := make([]openapi.Stop, 0, len(inForce))
	for _, stop := range inForce {
		items = append(items, stopFrom(stop))
	}
	return openapi.ListStops200JSONResponse{Items: items}, nil
}

/*
SetStop throws a switch or takes it off.

Stopping needs less authority than starting, on purpose. Anybody who can read
runs in a scope may stop it; only somebody who can cause runs may start it
again. The two mistakes are not symmetric: over-permitting a stop makes the
platform go quiet, which is loud, visible and reversible in one press, while
over-permitting a start causes runs nobody authorised. The person who needs
this at 3am is on call, and may not be the person who writes specifications.

It is checked in the scope the switch reaches — an area stop inside that area,
an installation stop in the scope that owns the installation.
*/
func (s *Server) SetStop(ctx context.Context, req openapi.SetStopRequestObject) (openapi.SetStopResponseObject, error) {
	stop, err := stopOf(req.Body)
	if err != nil {
		return openapi.SetStop400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Invalid stop", err.Error())),
		}, nil
	}

	where := stop.Scope
	if stop.Level != domain.StopScope {
		where = adminScope
	}
	needed := domain.PermRunRead
	if !req.Body.Stopped {
		needed = domain.PermRunTrigger
	}
	if err := auth.Require(ctx, needed, where); err != nil {
		return openapi.SetStop403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(needed, where),
		}, nil
	}
	if s.stops == nil {
		return nil, errors.New("this installation has no administration store")
	}

	caller, _ := auth.PrincipalFrom(ctx)
	stop.By = caller.ID

	act := s.stops.Start
	if req.Body.Stopped {
		act = s.stops.Stop
	}
	if err := act(ctx, stop); err != nil {
		return openapi.SetStop400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "The switch could not be set", err.Error())),
		}, nil
	}
	return openapi.SetStop204Response{}, nil
}

// stopOf reads the request, refusing a target that names nothing.
func stopOf(body *openapi.SetStopJSONRequestBody) (domain.Stop, error) {
	if body == nil {
		return domain.Stop{}, errors.New("a stop needs a level")
	}
	stop := domain.Stop{Level: domain.StopLevel(body.Level)}
	if !stop.Level.Valid() {
		return domain.Stop{}, fmt.Errorf("%q is not a level of stop", body.Level)
	}
	if body.Reason != nil {
		stop.Reason = *body.Reason
	}

	switch stop.Level {
	case domain.StopScope:
		if body.Scope == nil || body.Scope.Company == "" {
			return domain.Stop{}, errors.New("a scope stop needs a company")
		}
		stop.Scope = domain.Scope{
			Company: domain.CompanyID(body.Scope.Company),
			Area:    domain.AreaID(body.Scope.Area),
		}
	case domain.StopAgent:
		if body.AgentId == nil || *body.AgentId == "" {
			return domain.Stop{}, errors.New("an agent stop needs an agent")
		}
		stop.Agent = domain.AgentID(*body.AgentId)
	}
	return stop, nil
}

func stopFrom(stop domain.Stop) openapi.Stop {
	out := openapi.Stop{
		Level:  openapi.StopLevel(stop.Level),
		Reason: stop.Reason,
	}
	if stop.Scope.Company != "" || stop.Scope.Area != "" {
		out.Scope = &openapi.Scope{
			Company: string(stop.Scope.Company), Area: string(stop.Scope.Area),
		}
	}
	if stop.Agent != "" {
		out.AgentId = ptr(string(stop.Agent))
	}
	if stop.By != "" {
		out.By = ptr(string(stop.By))
	}
	if !stop.At.IsZero() {
		out.At = ptr(stop.At)
	}
	return out
}
