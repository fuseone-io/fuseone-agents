package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

// Publisher writes a version and records whether the agent may run.
//
// Declared here by the consumer, and deliberately two things: publishing is
// authorship and pausing is operations, and an interface that made them one
// call would invite a screen that starts an agent by saving it.
type Publisher interface {
	// Publish judges the event cycles the version closes under the same
	// serialisation as its write, and answers with them (#227).
	Publish(ctx context.Context, p spec.Publication) (spec.Published, error)
	SetPaused(ctx context.Context, agent domain.AgentID, paused bool, by domain.UserID) error
	IsPaused(ctx context.Context, agent domain.AgentID) (bool, error)
}

// WithPublisher wires authoring.
func (s *Server) WithPublisher(p Publisher) *Server {
	s.publisher = p
	return s
}

// PublishAgent writes the next version of an agent.
//
// Rendered to the file format and parsed back, so a version stays the digest
// of its bytes: the same definition typed here and written in an editor
// produce the same version, and publishing identical text twice returns the
// version it already had rather than making a second one of the same words.
func (s *Server) PublishAgent(
	ctx context.Context, req openapi.PublishAgentRequestObject,
) (openapi.PublishAgentResponseObject, error) {
	guard, bad := agentGuard(ctx, req.Params.IfMatch, req.Params.IfNoneMatch)
	if bad != "" {
		return publishInvalid(bad), nil
	}
	draft, problem := draftOf(req)
	if problem != nil {
		return problem, nil
	}
	// Publishing into an area needs the right to publish into the exact
	// company/area pair the author chose. Area identifiers are scoped by
	// company; inferring the company from the first matching grant is how an
	// editor showing acme/platform can publish default/platform.
	scope, allowed := publishScope(ctx, draft.Company, draft.Area)
	if !allowed {
		return openapi.PublishAgent403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, scope),
		}, nil
	}
	if s.publisher == nil {
		return nil, errNoAdministration
	}

	definition, published, err := canonical(draft)
	if err != nil {
		return publishInvalid(err.Error()), nil
	}

	detail, err := s.checker().Check(ctx, published, clockOr(s.clock).Now())
	if err != nil {
		return nil, err
	}
	if detail != "" {
		return publishInvalid(detail), nil
	}

	before, err := s.agentVersions(ctx, domain.AgentID(req.AgentId))
	if err != nil {
		return nil, err
	}

	judged, err := s.publisher.Publish(ctx, spec.Publication{
		Spec: published, By: callerOf(ctx), Company: scope.Company,
		RefuseStages: refusedStages(req.Params.RefuseIfStage),
		Guard:        guard,
	})
	if refused, ok := publicationRefused(req.AgentId, err); ok {
		return refused, nil
	}
	if err != nil {
		return nil, fmt.Errorf("publish %s: %w", req.AgentId, err)
	}
	return openapi.PublishAgent200JSONResponse{
		AgentId: string(published.ID), VersionId: string(published.Version),
		Created:           !before[published.Version],
		Paused:            judged.Paused,
		Definition:        ptr(string(definition)),
		EventCycleWarning: eventCycleWarning(judged, auth.VisibleScopes(ctx, domain.PermAgentRead)),
	}, nil
}

// SetAgentPaused starts or stops an agent.
func (s *Server) SetAgentPaused(
	ctx context.Context, req openapi.SetAgentPausedRequestObject,
) (openapi.SetAgentPausedResponseObject, error) {
	current, ok := s.currentAgent(ctx, domain.AgentID(req.AgentId))
	if !ok || s.publisher == nil {
		return openapi.SetAgentPaused404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
		}, nil
	}
	scope := current.Scope
	// Starting an agent is causing every run it will make, so it needs the
	// authority to cause runs rather than the one to write definitions.
	if err := auth.Require(ctx, domain.PermRunTrigger, scope); err != nil {
		return openapi.SetAgentPaused403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermRunTrigger, scope),
		}, nil
	}

	// Only a start is checked. Stopping an agent must always be possible —
	// a gate that can refuse it is a gate that keeps a broken agent running.
	if !req.Body.Paused {
		// An agent out of circulation is not startable, whatever else is
		// true. Retiring stops it; starting it again from a screen that does
		// not list it would be a run nobody is watching for.
		if retired, err := s.isRetired(ctx, current.ID); err != nil {
			return nil, err
		} else if retired {
			return openapi.SetAgentPaused409ApplicationProblemPlusJSONResponse(
				conflicted(fmt.Sprintf(
					"%s is retired; bring it back before starting it", current.ID))), nil
		}

		broken, err := s.brokenCases(ctx, current.ID, current.VersionID)
		if err != nil {
			return nil, err
		}
		if len(broken) > 0 {
			return openapi.SetAgentPaused409ApplicationProblemPlusJSONResponse(
				conflicted(refusedForCorpus(current.ID, broken))), nil
		}
	}

	if err := s.publisher.SetPaused(
		ctx, domain.AgentID(req.AgentId), req.Body.Paused, callerOf(ctx),
	); err != nil {
		return nil, fmt.Errorf("set %s paused: %w", req.AgentId, err)
	}
	return openapi.SetAgentPaused204Response{}, nil
}

// refusedStages is the publication's precondition: absent refuses nothing.
func refusedStages(in *openapi.Stage) []domain.Stage {
	if in == nil {
		return nil
	}
	return []domain.Stage{domain.StageOf(string(*in))}
}

// publishScope checks the exact scope the caller asked to publish into.
//
// A grant with no area covers its whole company, which is how somebody who
// administers a company publishes into an area nobody has granted separately.
func publishScope(
	ctx context.Context, company domain.CompanyID, area domain.AreaID,
) (domain.Scope, bool) {
	target := domain.Scope{Company: company, Area: area}
	if company == "" || company == domain.Installation || area == "" {
		return target, false
	}
	if err := domain.ValidCompanyID(string(company)); err != nil {
		return target, false
	}
	for _, held := range auth.VisibleScopes(ctx, domain.PermAgentPublish) {
		if held.Contains(target) {
			return target, true
		}
	}
	return target, false
}

// agentVersions is which versions already exist, so publishing can report
// whether it made one.
func (s *Server) agentVersions(ctx context.Context, agent domain.AgentID) (map[domain.VersionID]bool, error) {
	out := map[domain.VersionID]bool{}
	if s.agents == nil {
		return out, nil
	}
	versions, err := s.agents.Versions(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("read versions of %s: %w", agent, err)
	}
	for _, v := range versions {
		out[v.VersionID] = true
	}
	return out, nil
}
