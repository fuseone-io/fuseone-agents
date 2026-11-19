package httpapi

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

/*
Activation (NT-018 phase 2, release 2c).

A candidate, or a version already published, becomes the agent's current version through
the publication's locks and checks. This route does what comes before any lock — the
headers by syntax, the version's scope, the content fence — and the publication's checks
over the stored definition; the store does the rest under the agent's row, in the order
NT-018 gives, moving the agent's schedules and webhooks in the same transaction, and the
route answers what it found.
*/

// Activations is what the activation route reads and does, declared here by the consumer.
type Activations interface {
	// Scopes is where the version lives, before any lock: none for a version the agent
	// does not have.
	Scopes(ctx context.Context, agent domain.AgentID, version domain.VersionID) ([]domain.Scope, error)
	// Definition is the version as stored, for the publication's checks: false when no
	// seal of a candidate is readable, which the activation answers under its locks.
	Definition(ctx context.Context, agent domain.AgentID, version domain.VersionID, scope domain.Scope) (spec.Spec, bool, error)
	Activate(ctx context.Context, a spec.Activation) (spec.Activated, error)
}

// WithActivations wires activation.
func (s *Server) WithActivations(a Activations) *Server {
	s.activations = a
	return s
}

type activationReply = openapi.ActivateAgentVersionResponseObject

// errNeedsNever is a definition with nothing in its never block.
var errNeedsNever = errors.New("activation needs a non-empty never block")

// ActivateAgentVersion makes a candidate, or a published version, current.
func (s *Server) ActivateAgentVersion(
	ctx context.Context, req openapi.ActivateAgentVersionRequestObject,
) (activationReply, error) {
	if req.Params.IfMatch == nil && req.Params.IfNoneMatch == nil {
		return activationRequired(), nil
	}
	guard, bad := agentGuard(ctx, req.Params.IfMatch, req.Params.IfNoneMatch)
	if bad != "" {
		return activationInvalid(CodeInvalidInput, bad), nil
	}
	agent, version := domain.AgentID(req.AgentId), domain.VersionID(req.Version)
	if s.activations == nil || s.regressions == nil || s.publisher == nil {
		return activationAbsent(req.AgentId), nil
	}
	scope, allowed, err := s.activationScope(ctx, agent, version)
	if err != nil || !allowed {
		return activationAbsent(req.AgentId), err
	}
	a := spec.Activation{Agent: agent, Version: version, Scope: scope, By: callerOf(ctx),
		Guard: guard, Accept: acceptNever}
	var (
		activated spec.Activated
		refused   activationReply
	)
	err = s.regressions.Hold(ctx, agent, func(ctx context.Context) error {
		var err error
		activated, refused, err = s.activateHeld(ctx, a)
		return err
	})
	if refused != nil || err != nil {
		return refused, err
	}
	return s.activationAnswered(ctx, activated), nil
}

/*
activationScope is the version's scope, authorised before any lock (step 2): the caller
must hold agent:publish on every scope a row of the version names, or the version does
not exist for them.
*/
func (s *Server) activationScope(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) (domain.Scope, bool, error) {
	scopes, err := s.activations.Scopes(ctx, agent, version)
	if err != nil || len(scopes) == 0 {
		return domain.Scope{}, false, err
	}
	for _, scope := range scopes {
		if auth.Require(ctx, domain.PermAgentPublish, scope) != nil {
			return domain.Scope{}, false, nil
		}
	}
	return scopes[0], true, nil
}

// activateHeld runs inside the content fence: the publication's checks over the stored
// definition (step 4), then the store's steps under the locks.
func (s *Server) activateHeld(ctx context.Context, a spec.Activation) (spec.Activated, activationReply, error) {
	definition, found, err := s.activations.Definition(ctx, a.Agent, a.Version, a.Scope)
	if err != nil {
		return spec.Activated{}, nil, err
	}
	// No readable seal is answered here, before the locks: activating bytes that were
	// never checked, because a seal of the same version was written in between, would
	// skip the publication's checks.
	if !found {
		return spec.Activated{}, activationUnreadable(a.Agent, a.Version), nil
	}
	detail, err := s.checker().Check(ctx, definition, clockOr(s.clock).Now())
	if err != nil || detail != "" {
		return spec.Activated{}, activationInvalid(CodeInvalidInput, detail), err
	}
	activated, err := s.activations.Activate(ctx, a)
	if refused, ok := activationRefused(string(a.Agent), err); ok {
		return spec.Activated{}, refused, nil
	}
	return activated, nil, err
}

/*
activationAnswered answers a committed activation. Nothing is left to do after the commit:
the schedules, the webhooks and the pause moved in the store's transaction, under the
agent's row, so a slower activation can never land its triggers after a later one.
*/
func (s *Server) activationAnswered(ctx context.Context, a spec.Activated) activationReply {
	out := openapi.ActivateAgentVersion200JSONResponse{
		Headers: openapi.ActivateAgentVersion200ResponseHeaders{ETag: string(a.Revision)},
		Body: openapi.AgentActivated{
			AgentId: string(a.Spec.ID), VersionId: string(a.Spec.Version), Created: a.Created, Paused: a.Paused,
			EventCycleWarning: eventCycleWarning(a.Judged, auth.VisibleScopes(ctx, domain.PermAgentRead)),
		},
	}
	if a.SealID != "" {
		out.Body.SealId = ptr(a.SealID)
	}
	return out
}

// acceptNever is the workbench's checklist on the definition the store activates.
func acceptNever(s spec.Spec) error {
	if !hasNever(blocks.Parse(s.Instructions)) {
		return errNeedsNever
	}
	return nil
}
