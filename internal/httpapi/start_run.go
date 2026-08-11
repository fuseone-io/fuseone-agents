package httpapi

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
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

	// The key answers first. A caller retrying after a timeout is doing the
	// right thing, and it must reach the run it already started.
	key := req.Params.IdempotencyKey
	if existing, err := s.store.RunByIdemKey(ctx, key); err == nil {
		run, _, err := s.project(ctx, existing)
		if err != nil {
			return nil, fmt.Errorf("project %s: %w", existing, err)
		}
		return openapi.StartRun200JSONResponse(run), nil
	}

	runID := domain.RunID(fmt.Sprintf("run_%s_%d", published.ID, s.now().UnixMilli()))

	inputRef, err := s.storeInput(ctx, runID, req.Body)
	if err != nil {
		return nil, err
	}

	step := domain.Step{
		RunID:      runID,
		Kind:       domain.StepRunStarted,
		Scope:      published.Scope,
		AgentID:    published.ID,
		VersionID:  published.VersionID,
		OnBehalfOf: callerOf(ctx),
		IdemKey:    key,
		At:         s.now(),
		Payload:    mustJSON(domain.RunStartedPayload{Trigger: "manual", InputRef: inputRef}),
	}
	if _, err := s.store.Append(ctx, step); err != nil {
		// Two requests with the same key raced, and this one lost. Asked
		// rather than pattern-matched on the error: whatever went wrong, if
		// the key now names a run then that run is the answer both callers
		// wanted, and if it does not the failure was real.
		if existing, lookupErr := s.store.RunByIdemKey(ctx, key); lookupErr == nil {
			run, _, projectErr := s.project(ctx, existing)
			if projectErr != nil {
				return nil, fmt.Errorf("project %s: %w", existing, projectErr)
			}
			return openapi.StartRun200JSONResponse(run), nil
		}
		return nil, fmt.Errorf("open run: %w", err)
	}

	run, _, err := s.project(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("project %s: %w", runID, err)
	}
	return openapi.StartRun201JSONResponse(run), nil
}

// callerOf is who asked. A run opened from the console is opened on somebody's
// behalf, and the trail has to say whose.
func callerOf(ctx context.Context) domain.UserID {
	if principal, ok := auth.PrincipalFrom(ctx); ok {
		return domain.UserID(principal.ID)
	}
	return ""
}

// storeInput puts what the run is about in the content store.
//
// Never in the ledger: a ticket, a message or a payload routinely carries
// personal data, and the ledger is kept for years and read by people who have
// no business seeing it (AU-04).
func (s *Server) storeInput(
	ctx context.Context, runID domain.RunID, body *openapi.StartRunJSONRequestBody,
) (string, error) {
	if body == nil || body.Input == nil || *body.Input == "" || s.content == nil {
		return "", nil
	}
	ref, err := s.content.Put(ctx, runID, domain.FirstSeq, []byte(*body.Input))
	if err != nil {
		return "", fmt.Errorf("store run input: %w", err)
	}
	return ref, nil
}

// now is injectable so a run's opening instant is a fact of the request rather
// than of whichever machine served it.
func (s *Server) now() time.Time {
	if s.clock != nil {
		return s.clock.Now()
	}
	return time.Now()
}
