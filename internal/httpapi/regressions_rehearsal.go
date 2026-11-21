package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/regression"
)

/*
A correction kept from a rehearsal (NT-018 phase 5b). The case keeps the world it was
corrected in: each call the rehearsal made with the answer its author gave, copied with the
input and recorded in one transaction (C2), so a battery replays them in order.

Only a rehearsal that ended with every call answered is kept (D5): a world with a hole in it
would replay a situation nobody corrected. The case goes to the rehearsal's sealed scope,
which is where its candidate was authored, so the agent need not be published; the caller
needs agent:publish and run:read there, and anything out of reach reads as absent (D3).
*/

// errRehearsalUnkept is a rehearsal that cannot be kept as a case, and why.
var errRehearsalUnkept = errors.New("this rehearsal cannot be kept as a case")

// rehearsalCall is one call of a rehearsal, and the answer its author gave, by reference.
type rehearsalCall struct {
	tool   domain.ToolID
	args   string
	answer string
}

// rehearsalOf answers the opening of a run that is a live rehearsal.
func rehearsalOf(steps []domain.Step) (domain.RunStartedPayload, bool) {
	if len(steps) == 0 || steps[0].Kind != domain.StepRunStarted {
		return domain.RunStartedPayload{}, false
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
		return domain.RunStartedPayload{}, false
	}
	return started, started.Simulated && started.Rehearsal
}

// recordFromRehearsal keeps a correction of a rehearsal, inside the corpus fence an erasure
// takes, reading the rehearsal again there.
func (s *Server) recordFromRehearsal(
	ctx context.Context, req openapi.RecordRegressionRequestObject,
) (openapi.RecordRegressionResponseObject, error) {
	var recorded domain.RegressionCase
	var refused error
	agent := domain.AgentID(req.AgentId)
	err := s.regressions.Hold(ctx, agent, func(ctx context.Context) error {
		c, copying, readErr := s.rehearsalCopy(ctx, agent, req.Body)
		if readErr != nil {
			refused = readErr
			return nil
		}
		recorded, refused = s.regressions.RecordCopying(ctx, c, copying)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return correctionOfRehearsal(req.AgentId, recorded, refused), nil
}

// correctionOfRehearsal says what recording a rehearsal came to.
func correctionOfRehearsal(agent string, recorded domain.RegressionCase, refused error) openapi.RecordRegressionResponseObject {
	switch {
	case refused == nil:
		return openapi.RecordRegression201JSONResponse(toRegressionCase(recorded))
	case errors.Is(refused, errRehearsalUnkept):
		return openapi.RecordRegression409ApplicationProblemPlusJSONResponse(conflicted(refused.Error()))
	case errors.Is(refused, errNoSuchRun), errors.Is(refused, regression.ErrDatasetElsewhere):
		return openapi.RecordRegression404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(agent)}
	}
	return openapi.RecordRegression400ApplicationProblemPlusJSONResponse(invalid(refused.Error()))
}

// errNoSuchRun is a rehearsal out of the caller's reach, or of another agent: absent.
var errNoSuchRun = errors.New("no such run")

// rehearsalCopy reads what a correction of a rehearsal copies: the case, its input and its
// world, each answer read back so one erased since refuses the recording.
func (s *Server) rehearsalCopy(
	ctx context.Context, agent domain.AgentID, body *openapi.RecordRegressionJSONRequestBody,
) (domain.RegressionCase, regression.Copying, error) {
	steps, err := s.store.Read(ctx, domain.RunID(body.RunId), domain.FirstSeq)
	started, ok := rehearsalOf(steps)
	if err != nil || !ok || steps[0].AgentID != agent {
		return domain.RegressionCase{}, regression.Copying{}, errNoSuchRun
	}
	scope := steps[0].Scope
	if auth.Require(ctx, domain.PermAgentPublish, scope) != nil || auth.Require(ctx, domain.PermRunRead, scope) != nil {
		return domain.RegressionCase{}, regression.Copying{}, errNoSuchRun
	}
	calls, err := endedWithEveryCallAnswered(steps)
	if err != nil {
		return domain.RegressionCase{}, regression.Copying{}, err
	}
	copying, err := s.readWorld(ctx, started.InputRef, calls)
	if err != nil {
		return domain.RegressionCase{}, regression.Copying{}, err
	}
	c := domain.RegressionCase{
		ID: regressionID(agent, body.RunId), Agent: agent, Scope: scope,
		Expectations: fromExpectations(body.Expectations), FromRun: domain.RunID(body.RunId),
		CreatedBy: callerOf(ctx),
	}
	if body.Note != nil {
		c.Note = *body.Note
	}
	return c, copying, nil
}

// readWorld reads back the input and every answer, refusing what has been erased since.
func (s *Server) readWorld(ctx context.Context, inputRef string, calls []rehearsalCall) (regression.Copying, error) {
	input, err := s.content.Get(ctx, inputRef)
	if inputRef == "" || err != nil {
		return regression.Copying{}, fmt.Errorf("%w: its case can no longer be read", errRehearsalUnkept)
	}
	world := &regression.WorldCopy{Answers: make([]regression.Answer, 0, len(calls))}
	for i, call := range calls {
		answer, err := s.content.Get(ctx, call.answer)
		if err != nil {
			return regression.Copying{}, fmt.Errorf("%w: answer %d can no longer be read", errRehearsalUnkept, i+1)
		}
		world.Answers = append(world.Answers, regression.Answer{Tool: call.tool, ArgsDigest: call.args, Bytes: answer})
	}
	return regression.Copying{Input: input, World: world}, nil
}

/*
endedWithEveryCallAnswered answers a rehearsal's calls in the order it made them, each with
the answer its author gave: a world_awaited naming the call, and a world_supplied naming that
wait. A rehearsal still going, or that ended with a call nobody answered, is refused.
*/
func endedWithEveryCallAnswered(steps []domain.Step) ([]rehearsalCall, error) {
	state, err := engine.Fold(steps)
	if err != nil || !state.Terminal() {
		return nil, fmt.Errorf("%w: it has not ended", errRehearsalUnkept)
	}
	calls := callsOf(steps)
	for i, call := range calls {
		if call.answer == "" {
			return nil, fmt.Errorf("%w: call %d (%s) was never answered", errRehearsalUnkept, i+1, call.tool)
		}
	}
	return calls, nil
}

// callsOf pairs each call a rehearsal made with the answer its author gave, if any.
func callsOf(steps []domain.Step) []rehearsalCall {
	calls, at, waits := []rehearsalCall{}, map[int64]int{}, map[int64]int{}
	for _, step := range steps {
		switch step.Kind {
		case domain.StepToolCalled:
			var p domain.ToolCalledPayload
			_ = json.Unmarshal(step.Payload, &p)
			at[step.Seq] = len(calls)
			calls = append(calls, rehearsalCall{tool: p.Tool, args: p.ArgsDigest})
		case domain.StepWorldAwaited:
			var p domain.WorldAwaitedPayload
			if json.Unmarshal(step.Payload, &p) == nil {
				if i, ok := at[p.CallSeq]; ok {
					waits[step.Seq] = i
				}
			}
		case domain.StepWorldSupplied:
			var p domain.WorldSuppliedPayload
			if json.Unmarshal(step.Payload, &p) == nil {
				if i, ok := waits[p.AtSeq]; ok {
					calls[i].answer = p.Ref
				}
			}
		}
	}
	return calls
}
