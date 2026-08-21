package httpapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

/*
Schedules is where a declared cron trigger becomes a moment the worker will
reach. Declared here by the consumer.

It exists because publishing from this screen did not reach it. Schedules were
reconciled from specification files at worker start-up, which is the right
thing for an installation that keeps its agents in git and nothing at all for
one that presses the button: the version recorded the trigger, the screen
printed it back, and no clock ever knew. Both ways of publishing end at the
same table now.
*/
type Schedules interface {
	Sync(ctx context.Context, agent domain.AgentID, schedules []string, from time.Time) error
}

// WithSchedules wires where a cron trigger is reconciled.
func (s *Server) WithSchedules(schedules Schedules) *Server {
	s.schedules = schedules
	return s
}

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
	// Publishing into an area needs the right to publish into the exact
	// company/area pair the author chose. Area identifiers are scoped by
	// company; inferring the company from the first matching grant is how an
	// editor showing acme/platform can publish default/platform.
	scope, allowed := publishScope(ctx,
		domain.CompanyID(req.Body.Company), domain.AreaID(req.Body.Area))
	if !allowed {
		return openapi.PublishAgent403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, scope),
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

	// Before the version is written, because a schedule nobody can parse
	// would otherwise be published, reported as saved, and never fire.
	if bad := unparseableSchedule(published, clockOr(s.clock).Now()); bad != "" {
		return openapi.PublishAgent400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(fmt.Sprintf("the schedule %q is not one this platform can read", bad))),
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

	// After the version exists, so what is reconciled is what was published.
	// An error here is returned rather than swallowed: publishing the same
	// definition again is a no-op, so a caller retrying reaches the sync.
	if s.schedules != nil {
		if err := s.schedules.Sync(ctx, published.ID,
			spec.CronSchedules(published), clockOr(s.clock).Now()); err != nil {
			return nil, fmt.Errorf("reconcile the schedules of %s: %w", published.ID, err)
		}
	}
	if s.webhooks != nil {
		if err := s.webhooks.Sync(ctx, published.ID, scope, spec.WebhookPaths(published)); err != nil {
			return nil, fmt.Errorf("reconcile the webhooks of %s: %w", published.ID, err)
		}
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

// renderAndParse turns the form's fields into the file, and the file into the
// specification. Both directions, because the file is the artefact and the
// version is its digest.
func renderAndParse(id string, in openapi.AgentDefinition) ([]byte, spec.Spec, error) {
	if id == "" {
		return nil, spec.Spec{}, errors.New("um agente precisa de um identificador")
	}

	draft := spec.Spec{
		ID: domain.AgentID(id), Name: in.Name,
		Company: domain.CompanyID(in.Company), Area: domain.AreaID(in.Area),
		Provider: in.Provider, Model: in.Model, Effort: valueOr(in.Effort),
		Instructions: in.Instructions,
	}
	for _, t := range valueOr(in.Tools) {
		draft.Tools = append(draft.Tools, domain.ToolID(t))
	}
	draft.Emits = emitsOf(in.Emits)
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
	draft.Steps = stepsOf(in.Steps)

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

func emitsOf(in *[]openapi.AgentEvent) spec.Emits {
	if in == nil {
		return nil
	}
	out := make(spec.Emits, 0, len(*in))
	for _, event := range *in {
		next := domain.AgentEvent{
			Event:   event.Event,
			Context: valueOr(event.Context),
		}
		if event.Artifacts != nil {
			next.Artifacts = append([]string(nil), *event.Artifacts...)
		}
		out = append(out, next)
	}
	return out
}

/*
stepsOf carries the declared steps into the specification.

They used to be dropped here, which was not a missing feature but a quiet
widening: `reaches` is what the Gate allows while a run sits at a step, so a
definition rendered without them hands every step the whole capability pack —
on an edit somebody made for an unrelated reason, with nothing on screen
saying so.
*/
func stepsOf(in *[]openapi.AgentStep) []spec.Step {
	if in == nil {
		return nil
	}
	out := make([]spec.Step, 0, len(*in))
	for _, step := range *in {
		next := spec.Step{
			Name:      step.Name,
			StopsWhen: valueOr(step.StopsWhen),
			Model:     valueOr(step.Model),
			Effort:    valueOr(step.Effort),
		}
		for _, tool := range valueOr(step.Reaches) {
			next.Reaches = append(next.Reaches, domain.ToolID(tool))
		}
		out = append(out, next)
	}
	return out
}
