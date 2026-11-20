package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Acting in a live rehearsal (NT-018 phase 4, release 4b): answering a call for the world,
deciding an action the Gate held, and ending the rehearsal.

Each route acts only on a live rehearsal, a run opened simulated and marked as one, and
only at the step it names. The check against the run as read is the message a stale tab
gets; the guarantee is the append, sealed only onto the head the request was judged
against, so a second answer, a decision overtaken or a run that moved writes nothing (an
ending whose release of the reservation landed keeps that release; see AbandonRehearsal).
A run outside the caller's agent:publish scopes is answered as one that does not exist.
*/

// maxWorldAnswerBytes bounds what an author answers for one call: room for a realistic
// tool result. The answer is kept whole, never truncated, so the bound is the route's.
const maxWorldAnswerBytes = 256 << 10

// rehearsalRefusal is why a request on a run is not about a live rehearsal.
type rehearsalRefusal int

const (
	rehearsalFound rehearsalRefusal = iota
	// rehearsalAbsent is a run that does not exist or that the caller may not act in.
	rehearsalAbsent
	// notARehearsal is a run the caller may act in that is not a live rehearsal.
	notARehearsal
)

// liveRun is a live rehearsal as read for one request.
type liveRun struct {
	steps []domain.Step
	state engine.State
}

func (r liveRun) head() domain.Step { return r.steps[len(r.steps)-1] }

// liveRehearsal reads the run and says whether it is a live rehearsal the caller may act in.
func (s *Server) liveRehearsal(ctx context.Context, id domain.RunID) (liveRun, rehearsalRefusal, error) {
	steps, state, err := s.trail(ctx, id)
	if err != nil || steps == nil {
		return liveRun{}, rehearsalAbsent, err
	}
	if auth.Require(ctx, domain.PermAgentPublish, state.Scope) != nil {
		return liveRun{}, rehearsalAbsent, nil
	}
	var started domain.RunStartedPayload
	if steps[0].Kind == domain.StepRunStarted {
		decodePayload(steps[0].Payload, &started)
	}
	if !started.Simulated || !started.Rehearsal {
		return liveRun{}, notARehearsal, nil
	}
	return liveRun{steps: steps, state: state}, rehearsalFound, nil
}

/*
AnswerForTheWorld records an author's answer to the call the rehearsal waits on: any author
who may publish in the rehearsal's scope, named in the step as the one who answered.

The run is read once to learn its agent, then judged again inside the agent's exclusive
fence, the one erasing the rehearsal's conversation or seal takes: an erasure either comes
first, and the answer joins a run already erased, or waits, and then reaches the answer.
The answer and the step that names it are kept in one transaction, sealed onto the wait,
so a refused or interrupted answer keeps nothing.
*/
func (s *Server) AnswerForTheWorld(
	ctx context.Context, req openapi.AnswerForTheWorldRequestObject,
) (openapi.AnswerForTheWorldResponseObject, error) {
	runID := domain.RunID(req.RunId)
	live, why, err := s.liveRehearsal(ctx, runID)
	if answer, refused := worldRefusal(req.RunId, why, err); refused {
		return answer, err
	}
	var answer openapi.AnswerForTheWorldResponseObject
	err = s.withinAgentFence(ctx, live.state.AgentID, func(ctx context.Context) error {
		var err error
		answer, err = s.answerFenced(ctx, runID, req.Body)
		return err
	})
	return answer, err
}

// worldRefusal is the answer a run that is not a live rehearsal the caller may act in gets.
func worldRefusal(id string, why rehearsalRefusal, err error) (openapi.AnswerForTheWorldResponseObject, bool) {
	switch {
	case err != nil:
		return nil, true
	case why == rehearsalAbsent:
		return openapi.AnswerForTheWorld404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(id)}, true
	case why == notARehearsal:
		return openapi.AnswerForTheWorld409ApplicationProblemPlusJSONResponse(conflicted("this run is not a live rehearsal")), true
	}
	return nil, false
}

// answerFenced judges the run again inside the fence and keeps the answer with its step.
func (s *Server) answerFenced(
	ctx context.Context, runID domain.RunID, body *openapi.WorldAnswer,
) (openapi.AnswerForTheWorldResponseObject, error) {
	live, why, err := s.liveRehearsal(ctx, runID)
	if answer, refused := worldRefusal(string(runID), why, err); refused {
		return answer, err
	}
	if why := worldAnswerRefusal(body); why != "" {
		return openapi.AnswerForTheWorld400ApplicationProblemPlusJSONResponse(invalid(why)), nil
	}
	head := live.head()
	if head.Kind != domain.StepWorldAwaited || head.Seq != body.AtSeq {
		return openapi.AnswerForTheWorld409ApplicationProblemPlusJSONResponse(conflicted(
			fmt.Sprintf("this rehearsal is not waiting for the world at step %d", body.AtSeq))), nil
	}
	switch err := s.keepAnswer(ctx, live, []byte(body.Answer)); {
	case errors.Is(err, domain.ErrHeadMoved):
		return openapi.AnswerForTheWorld409ApplicationProblemPlusJSONResponse(
			conflicted("this wait was answered already, or the rehearsal moved on")), nil
	case errors.Is(err, domain.ErrContentErased):
		return openapi.AnswerForTheWorld409ApplicationProblemPlusJSONResponse(
			conflicted("this answer was erased from this rehearsal; it cannot be given again")), nil
	case err != nil:
		return nil, fmt.Errorf("record the answer for %s: %w", runID, err)
	}
	run, _, err := s.project(ctx, runID)
	if err != nil {
		return nil, err
	}
	return openapi.AnswerForTheWorld200JSONResponse(run), nil
}

func worldAnswerRefusal(body *openapi.WorldAnswer) string {
	switch {
	case body == nil || body.Answer == "":
		return "an answer for the world needs some content"
	case len(body.Answer) > maxWorldAnswerBytes:
		return fmt.Sprintf("an answer for the world is at most %d bytes", maxWorldAnswerBytes)
	}
	return ""
}

// answerKeeper keeps an answer and the step that names it in one transaction.
type answerKeeper interface {
	AppendIfHeadWithContent(
		ctx context.Context, head domain.StepRef, run domain.RunID, seq int64, data []byte,
		build func(ref, digest string) domain.Step,
	) (domain.Step, error)
}

// keepAnswer keeps the answer, owned by the run, with the step that answers the wait.
func (s *Server) keepAnswer(ctx context.Context, live liveRun, answer []byte) error {
	keeper, ok := s.store.(answerKeeper)
	if !ok {
		return fmt.Errorf("this ledger cannot keep an answer with its step")
	}
	head, state := live.head(), live.state
	_, err := keeper.AppendIfHeadWithContent(ctx, domain.StepRef{Seq: head.Seq, Kind: head.Kind},
		state.RunID, head.Seq+1, answer, func(ref, digest string) domain.Step {
			return domain.Step{
				RunID: state.RunID, Scope: state.Scope, AgentID: state.AgentID, VersionID: state.VersionID,
				OnBehalfOf: state.OnBehalfOf, Kind: domain.StepWorldSupplied,
				At:      decidedAt(live.steps, clockOr(s.clock).Now()),
				IdemKey: fmt.Sprintf("world:%s:%d", state.RunID, head.Seq),
				Labels:  head.Labels.Union(domain.NewLabels(domain.LabelUntrusted)),
				Payload: mustJSON(domain.WorldSuppliedPayload{
					AtSeq: head.Seq, Ref: ref, Digest: digest, Bytes: int64(len(answer)), By: callerOf(ctx),
				}),
			}
		})
	return err
}
