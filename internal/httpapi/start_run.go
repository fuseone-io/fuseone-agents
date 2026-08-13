package httpapi

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/trigger"
)

// StartRun opens a run and returns. A worker picks it up.
//
// Nothing is executed inside the request: a tool call can take minutes and an
// approval can take hours, and an HTTP handler is the wrong place to hold
// either. What this does is append the one step that makes the run exist and
// claimable, pinned to the version published now — so a version published a
// second later never changes what this run does.
func (s *Server) StartRun(ctx context.Context, req openapi.StartRunRequestObject) (openapi.StartRunResponseObject, error) {
	absent := openapi.StartRun404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}
	if s.agents == nil {
		return absent, nil
	}

	versions, err := s.agents.Versions(ctx, domain.AgentID(req.AgentId))
	if err != nil {
		return nil, fmt.Errorf("agent versions: %w", err)
	}
	if len(versions) == 0 || !readable(versions[0].Scope, auth.VisibleScopes(ctx, domain.PermAgentRead)) {
		return absent, nil
	}
	published := versions[0]

	// Reading what an agent did and making it do something again are separate
	// authorities. An auditor holds the first and never the second.
	if err := auth.Require(ctx, domain.PermRunTrigger, published.Scope); err != nil {
		return openapi.StartRun403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermRunTrigger, published.Scope),
		}, nil
	}

	// The same opener the scheduler uses. Two paths that both "just append
	// run_started" drift, and the way they drift is that one of them forgets
	// the idempotency key and opens a run on every retry.
	opened, err := s.opener().Open(ctx, trigger.Request{
		Agent:   published.ID,
		IdemKey: req.Params.IdempotencyKey,
		Trigger: "manual",
		By:      callerOf(ctx),
		Input:   inputOf(req.Body),
	})
	if err != nil {
		return nil, fmt.Errorf("open run: %w", err)
	}

	run, _, err := s.project(ctx, opened.RunID)
	if err != nil {
		return nil, fmt.Errorf("project %s: %w", opened.RunID, err)
	}
	if opened.Created {
		return openapi.StartRun201JSONResponse(run), nil
	}
	// Not an error: a caller retrying after a timeout is doing the right thing,
	// and it must reach the run it already started.
	return openapi.StartRun200JSONResponse(run), nil
}

// opener builds the shared run opener over this server's ports.
func (s *Server) opener() *trigger.Opener {
	opener := trigger.NewOpener(s.store, s.agents, clockOr(s.clock))
	if s.content != nil {
		opener = opener.WithContent(s.content)
	}
	if s.stages != nil {
		// A draft may be simulated and may not act, by any route including
		// this button.
		opener = opener.WithStages(s.stages)
	}
	if s.pauses != nil {
		// Including the button. If a person could run a paused agent by
		// pressing something, "paused" would mean "does not run by itself",
		// which is not what the word says.
		opener = opener.WithPauses(s.pauses)
	}
	return opener
}

func clockOr(clock Clock) trigger.Clock {
	if clock != nil {
		return clock
	}
	return systemClock{}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func inputOf(body *openapi.StartRunJSONRequestBody) []byte {
	if body == nil || body.Input == nil {
		return nil
	}
	return []byte(*body.Input)
}

// callerOf is who asked. A run opened from the console is opened on somebody's
// behalf, and the trail has to say whose.
func callerOf(ctx context.Context) domain.UserID {
	if principal, ok := auth.PrincipalFrom(ctx); ok {
		return domain.UserID(principal.ID)
	}
	return ""
}
