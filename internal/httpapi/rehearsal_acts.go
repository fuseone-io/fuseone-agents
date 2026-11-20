package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
against, so a second answer, a decision overtaken or a run that moved writes nothing.
A run outside the caller's agent:publish scopes is answered as one that does not exist.
*/

// maxWorldAnswerBytes bounds what an author answers for one call: room for a realistic
// tool result, and far below the content store's own ceiling, so nothing is truncated.
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
AnswerForTheWorld records the author's answer to the call the rehearsal waits on.

The answer is kept first, owned by the run, and the step then names it by reference and
digest. Two answers racing to one wait both keep their bytes under the run, and only one
step is sealed: the loser's bytes are the run's like everything else it holds, reached by
every erasure that reaches the run.
*/
func (s *Server) AnswerForTheWorld(
	ctx context.Context, req openapi.AnswerForTheWorldRequestObject,
) (openapi.AnswerForTheWorldResponseObject, error) {
	runID := domain.RunID(req.RunId)
	live, why, err := s.liveRehearsal(ctx, runID)
	switch {
	case err != nil:
		return nil, err
	case why == rehearsalAbsent:
		return openapi.AnswerForTheWorld404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId)}, nil
	case why == notARehearsal:
		return openapi.AnswerForTheWorld409ApplicationProblemPlusJSONResponse(conflicted("this run is not a live rehearsal")), nil
	}
	if why := worldAnswerRefusal(req.Body); why != "" {
		return openapi.AnswerForTheWorld400ApplicationProblemPlusJSONResponse(invalid(why)), nil
	}
	head := live.head()
	if head.Kind != domain.StepWorldAwaited || head.Seq != req.Body.AtSeq {
		return openapi.AnswerForTheWorld409ApplicationProblemPlusJSONResponse(conflicted(
			fmt.Sprintf("this rehearsal is not waiting for the world at step %d", req.Body.AtSeq))), nil
	}
	step, err := s.suppliedStep(ctx, live, []byte(req.Body.Answer))
	if err != nil {
		return nil, err
	}
	switch _, err := s.store.AppendIfHead(ctx, domain.StepRef{Seq: head.Seq, Kind: head.Kind}, step); {
	case errors.Is(err, domain.ErrHeadMoved):
		return openapi.AnswerForTheWorld409ApplicationProblemPlusJSONResponse(
			conflicted("this wait was answered already, or the rehearsal moved on")), nil
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

// suppliedStep keeps the answer and names it in the step that answers the wait at the head.
func (s *Server) suppliedStep(ctx context.Context, live liveRun, answer []byte) (domain.Step, error) {
	if s.content == nil {
		return domain.Step{}, fmt.Errorf("no content store to keep an answer for the world")
	}
	head, state := live.head(), live.state
	ref, err := s.content.Put(ctx, state.RunID, head.Seq+1, answer)
	if err != nil {
		return domain.Step{}, fmt.Errorf("keep the answer for %s: %w", state.RunID, err)
	}
	sum := sha256.Sum256(answer)
	return domain.Step{
		RunID: state.RunID, Scope: state.Scope, AgentID: state.AgentID, VersionID: state.VersionID,
		OnBehalfOf: state.OnBehalfOf, Kind: domain.StepWorldSupplied,
		At:      decidedAt(live.steps, clockOr(s.clock).Now()),
		IdemKey: fmt.Sprintf("world:%s:%d", state.RunID, head.Seq),
		Labels:  head.Labels.Union(domain.NewLabels(domain.LabelUntrusted)),
		Payload: mustJSON(domain.WorldSuppliedPayload{
			AtSeq: head.Seq, Ref: ref, Digest: hex.EncodeToString(sum[:]),
			Bytes: int64(len(answer)), By: callerOf(ctx),
		}),
	}, nil
}
