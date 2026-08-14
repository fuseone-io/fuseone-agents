package httpapi

import (
	"context"
	"errors"
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
	Publish(ctx context.Context, s spec.Spec, by domain.UserID, company domain.CompanyID) error
	EnsurePaused(ctx context.Context, agent domain.AgentID, by domain.UserID) error
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
	// Publishing into an area needs the right to publish there. The company
	// comes from the caller's own grant rather than a constant: in phase 1
	// there is one, and hardcoding it is how a phase-2 bug gets written today
	// and found in a year.
	scope, allowed := publishScope(ctx, domain.AreaID(req.Body.Area))
	if !allowed {
		return openapi.PublishAgent403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish,
				domain.Scope{Area: domain.AreaID(req.Body.Area)}),
		}, nil
	}
	if s.publisher == nil {
		return nil, errNoAdministration
	}

	definition, published, err := renderAndParse(req.AgentId, *req.Body)
	if err != nil {
		return openapi.PublishAgent400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}

	before, err := s.agentVersions(ctx, domain.AgentID(req.AgentId))
	if err != nil {
		return nil, err
	}

	if err := s.publisher.Publish(ctx, published, callerOf(ctx), scope.Company); err != nil {
		return nil, fmt.Errorf("publish %s: %w", req.AgentId, err)
	}
	// Recorded as paused only if nobody has decided: republishing an agent
	// somebody deliberately started must not stop it.
	if err := s.publisher.EnsurePaused(ctx, published.ID, callerOf(ctx)); err != nil {
		return nil, err
	}

	paused, err := s.publisher.IsPaused(ctx, published.ID)
	if err != nil {
		return nil, err
	}

	return openapi.PublishAgent200JSONResponse{
		AgentId: string(published.ID), VersionId: string(published.Version),
		Created:    !before[published.Version],
		Paused:     paused,
		Definition: ptr(string(definition)),
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

// publishScope finds a grant that lets the caller publish into an area, and
// the company that grant belongs to.
//
// A grant with no area covers its whole company, which is how somebody who
// administers a company publishes into an area nobody has granted separately.
func publishScope(ctx context.Context, area domain.AreaID) (domain.Scope, bool) {
	for _, held := range auth.VisibleScopes(ctx, domain.PermAgentPublish) {
		if held.Area == "" || held.Area == area {
			return domain.Scope{Company: held.Company, Area: area}, true
		}
	}
	return domain.Scope{}, false
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

// renderAndParse turns the form's fields into the file, and the file into the
// specification. Both directions, because the file is the artefact and the
// version is its digest.
func renderAndParse(id string, in openapi.AgentDefinition) ([]byte, spec.Spec, error) {
	if id == "" {
		return nil, spec.Spec{}, errors.New("um agente precisa de um identificador")
	}

	draft := spec.Spec{
		ID: domain.AgentID(id), Name: in.Name, Area: domain.AreaID(in.Area),
		Provider: in.Provider, Model: in.Model, Effort: valueOr(in.Effort),
		Instructions: in.Instructions,
	}
	for _, t := range valueOr(in.Tools) {
		draft.Tools = append(draft.Tools, domain.ToolID(t))
	}
	draft.Emits = valueOr(in.Emits)
	if in.Budget != nil {
		draft.Budget = domain.Budget{
			Micros: valueOr(in.Budget.Micros), Tokens: valueOr(in.Budget.Tokens),
			ToolCalls: valueOr(in.Budget.ToolCalls), Steps: valueOr(in.Budget.Steps),
			WallClockMS: valueOr(in.Budget.WallClockMs),
		}
	}
	for _, t := range valueOr(in.Triggers) {
		draft.Triggers = append(draft.Triggers, spec.Trigger{
			Type: string(t.Type), Schedule: valueOr(t.Schedule),
			Path: valueOr(t.Path), Event: valueOr(t.Event),
		})
	}

	rendered, err := spec.Render(draft)
	if err != nil {
		return nil, spec.Spec{}, err
	}
	// Parsed back rather than trusted: validation lives in the parser, and a
	// console that skipped it would accept definitions a file could not.
	parsed, err := spec.Parse("console", rendered)
	if err != nil {
		return nil, spec.Spec{}, err
	}
	return rendered, parsed, nil
}
