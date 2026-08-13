package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/compensate"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Undoing a run: seeing the plan, and deciding.

Two operations because they are two decisions. Reading what would be undone
changes nothing and needs only the trail; performing it calls real tools
against real systems, and nobody should discover what those were by pressing
the button.

Neither of these performs anything. The abandonment records the decision and a
worker carries it out — an undo can take minutes, and a request handler is the
wrong place to hold one.
*/

// GetCompensation is what undoing this run would do, in the order it would
// happen.
func (s *Server) GetCompensation(ctx context.Context, req openapi.GetCompensationRequestObject) (openapi.GetCompensationResponseObject, error) {
	steps, state, err := s.trail(ctx, domain.RunID(req.RunId))
	if err != nil {
		return nil, err
	}
	if steps == nil {
		return openapi.GetCompensation404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
		}, nil
	}
	if err := auth.Require(ctx, domain.PermRunRead, state.Scope); err != nil {
		return openapi.GetCompensation403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermRunRead, state.Scope),
		}, nil
	}

	undos, err := s.undos(ctx)
	if err != nil {
		return nil, err
	}
	return openapi.GetCompensation200JSONResponse{
		Acts: actsFrom(compensate.Plan(steps, undos)),
	}, nil
}

/*
AbandonRun ends a run and asks for what it left standing to be undone.

Always a person's decision. A parked run is paused, not failed: compensating
one because the machine got stuck would undo work somebody was about to resume
by raising a ceiling.
*/
func (s *Server) AbandonRun(ctx context.Context, req openapi.AbandonRunRequestObject) (openapi.AbandonRunResponseObject, error) {
	runID := domain.RunID(req.RunId)
	steps, state, err := s.trail(ctx, runID)
	if err != nil {
		return nil, err
	}
	if steps == nil {
		return openapi.AbandonRun404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
		}, nil
	}

	// Ending a run makes the platform act on the world. That is the authority
	// to make it run, not the authority to read what it did.
	if err := auth.Require(ctx, domain.PermRunTrigger, state.Scope); err != nil {
		return openapi.AbandonRun403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermRunTrigger, state.Scope),
		}, nil
	}
	if state.Terminal() {
		return openapi.AbandonRun409ApplicationProblemPlusJSONResponse(problem(
			http.StatusConflict, "The run has already ended",
			fmt.Sprintf("Run %s is %s; there is nothing left to abandon",
				req.RunId, state.Phase))), nil
	}

	// Undoing unless told otherwise. Somebody ending a run mid-flight almost
	// always wants what it did taken back, and the other answer is the one
	// worth having to say out loud.
	wants := req.Body.Compensate == nil || *req.Body.Compensate

	undos, err := s.undos(ctx)
	if err != nil {
		return nil, err
	}
	plan := compensate.Plan(steps, undos)

	if err := s.recordAbandonment(ctx, state, req.Body.Reason, wants); err != nil {
		return nil, err
	}

	// What is going to be attempted, not what happened: the worker has not run
	// yet. The run's own stream is where each undo lands as it completes.
	outcomes := make([]struct {
		Act  openapi.CompensationAct `json:"act"`
		Done bool                    `json:"done"`
		Why  *string                 `json:"why,omitempty"`
	}, 0, len(plan))
	for _, act := range actsFrom(plan) {
		outcome := struct {
			Act  openapi.CompensationAct `json:"act"`
			Done bool                    `json:"done"`
			Why  *string                 `json:"why,omitempty"`
		}{Act: act}
		if !wants {
			outcome.Why = ptr("the run was abandoned without compensating")
		} else if act.Undo == nil {
			outcome.Why = ptr("nothing undoes this tool")
		}
		outcomes = append(outcomes, outcome)
	}
	return openapi.AbandonRun200JSONResponse{Outcomes: outcomes}, nil
}

// recordAbandonment appends the one step that ends the run.
func (s *Server) recordAbandonment(
	ctx context.Context, state engine.State, reason string, compensating bool,
) error {
	_, err := s.store.Append(ctx, domain.Step{
		RunID: state.RunID, Kind: domain.StepAbandoned, Scope: state.Scope,
		AgentID: state.AgentID, VersionID: state.VersionID,
		OnBehalfOf: state.OnBehalfOf, At: clockOr(s.clock).Now(),
		Payload: mustJSON(domain.AbandonedPayload{
			By: callerOf(ctx), Reason: reason, Compensate: compensating,
		}),
	})
	if err != nil {
		return fmt.Errorf("abandon %s: %w", state.RunID, err)
	}
	return nil
}

// trail reads a run and folds it. A nil slice means no such run.
func (s *Server) trail(ctx context.Context, id domain.RunID) ([]domain.Step, engine.State, error) {
	steps, err := s.store.Read(ctx, id, domain.FirstSeq)
	if err != nil {
		if isNotFound(err) {
			return nil, engine.State{}, nil
		}
		return nil, engine.State{}, err
	}
	if len(steps) == 0 {
		return nil, engine.State{}, nil
	}
	state, err := engine.Fold(steps)
	if err != nil {
		return nil, engine.State{}, err
	}
	return steps, state, nil
}

// undos is the Curator's ruling on what takes each tool back.
//
// An installation that has ruled on nothing gets an empty one and every act
// comes back with no undo — which the screen reports as work for a person,
// rather than as a run with nothing left standing.
func (s *Server) undos(ctx context.Context) (compensate.Catalogue, error) {
	if s.tools == nil {
		return undoMap{}, nil
	}
	entries, err := s.tools.Tools(ctx)
	if err != nil {
		return nil, fmt.Errorf("tool catalogue: %w", err)
	}
	out := make(undoMap, len(entries))
	for _, e := range entries {
		if e.CompensatedBy != "" {
			out[e.ID] = e.CompensatedBy
		}
	}
	return out, nil
}

type undoMap map[domain.ToolID]domain.ToolID

func (m undoMap) CompensatedBy(tool domain.ToolID) (domain.ToolID, bool) {
	undo, ok := m[tool]
	return undo, ok
}

func actsFrom(plan []compensate.Act) []openapi.CompensationAct {
	out := make([]openapi.CompensationAct, 0, len(plan))
	for _, act := range plan {
		entry := openapi.CompensationAct{Tool: string(act.Tool), Seq: act.Seq}
		if act.Undo != "" {
			entry.Undo = ptr(string(act.Undo))
		}
		out = append(out, entry)
	}
	return out
}
