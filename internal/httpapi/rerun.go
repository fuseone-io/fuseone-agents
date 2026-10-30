package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/trigger"
)

/*
Running again from a recorded input (#16).

Distinct from Replay on purpose. Replay asks whether the recorded decisions
would be made the same way and executes nothing; this opens a new run, with a
new id and a new chain, whose input is the one a recorded run was opened with.
The recorded run is read and never written.

What travels is the input and the labels it was recorded under, and nothing
else. The source's approvals, reservations, leases, idempotency key and stage
outcome are facts about the source; the new run earns its own through the
Gate like any other. The conversation a channel ask arrived in does not travel
either: a reply address is a recipient, and an operator pressing a button in
the console did not choose to answer in somebody's thread.
*/

// rerunRefusal is an answer that ends the request before anything is opened.
type rerunRefusal = openapi.RunFromInputResponseObject

// RunFromInput opens a new run seeded with a recorded run's input.
func (s *Server) RunFromInput(
	ctx context.Context, req openapi.RunFromInputRequestObject,
) (openapi.RunFromInputResponseObject, error) {
	source, refused, err := s.recordedSource(ctx, domain.RunID(req.RunId))
	if refused != nil || err != nil {
		return refused, err
	}
	target, refused, err := s.rerunTarget(ctx, source, versionAsked(req.Body))
	if refused != nil || err != nil {
		return refused, err
	}
	input, refused, err := s.recordedInput(ctx, source)
	if refused != nil || err != nil {
		return refused, err
	}

	var payload domain.RunStartedPayload
	decodePayload(source.Payload, &payload)
	opened, err := s.opener().Open(ctx, trigger.Request{
		Agent: target.ID, Version: target.VersionID,
		IdemKey: trigger.RerunKey(source.RunID, req.Params.IdempotencyKey),
		// The trigger is what the input is: the transcript reads a channel
		// ask through a projection that keeps its routing envelope from the
		// model, and relabelling it would hand the model that envelope.
		Trigger: payload.Trigger, By: callerOf(ctx), Input: input,
		Labels: source.Labels.Clone(), SourceRun: source.RunID,
	})
	if refused := openRefusal(err); refused != nil {
		return refused, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open run from %s: %w", source.RunID, err)
	}
	return s.rerunAnswer(ctx, opened)
}

/*
recordedSource is the source run's opening step, or the answer an absent run
gets.

A run the caller cannot read answers exactly as a run that does not exist,
the rule every other run read follows: confirming a run exists in an area
somebody cannot see is information about that area.
*/
func (s *Server) recordedSource(ctx context.Context, id domain.RunID) (domain.Step, rerunRefusal, error) {
	absent := openapi.RunFromInput404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(string(id)),
	}
	steps, err := s.store.Read(ctx, id, domain.FirstSeq)
	if isNotFound(err) || errors.Is(err, domain.ErrRunNotFound) {
		return domain.Step{}, absent, nil
	}
	if err != nil {
		return domain.Step{}, nil, fmt.Errorf("read %s: %w", id, err)
	}
	if len(steps) == 0 || !mayRead(ctx, domain.PermRunRead, steps[0].Scope) {
		return domain.Step{}, absent, nil
	}
	if steps[0].Kind != domain.StepRunStarted {
		return domain.Step{}, nil, fmt.Errorf("run %s does not open with run_started", id)
	}
	return steps[0], nil, nil
}

/*
rerunTarget is the version the new run is pinned to, once the caller may start
runs there.

The scope must be the source's own. An agent is not supposed to move, and if
one did, the recorded input carries the data of the scope it was asked in —
the opener's data barrier would refuse it anyway, and this says so plainly
without naming where the agent went.
*/
func (s *Server) rerunTarget(
	ctx context.Context, source domain.Step, asked domain.VersionID,
) (domain.AgentSummary, rerunRefusal, error) {
	if s.agents == nil {
		return domain.AgentSummary{}, nil, fmt.Errorf("no agent registry")
	}
	versions, err := s.agents.Versions(ctx, source.AgentID)
	if err != nil {
		return domain.AgentSummary{}, nil, fmt.Errorf("agent versions: %w", err)
	}
	target, err := trigger.Pin(versions, asked)
	if err != nil {
		return domain.AgentSummary{}, openapi.RunFromInput400ApplicationProblemPlusJSONResponse(
			invalid(fmt.Sprintf("no published version %q of %s", asked, source.AgentID))), nil
	}
	if target.Scope != source.Scope {
		return domain.AgentSummary{}, openapi.RunFromInput409ApplicationProblemPlusJSONResponse(
			conflicted("the agent no longer runs in the scope this run was recorded in")), nil
	}
	if err := auth.Require(ctx, domain.PermRunTrigger, target.Scope); err != nil {
		return domain.AgentSummary{}, openapi.RunFromInput403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermRunTrigger, target.Scope),
		}, nil
	}
	return target, nil, nil
}

/*
recordedInput is the bytes the source run was opened with, read here.

The client never names content: it names a run, and the reference comes from
that run's own opening step. Erased or expired content refuses rather than
opening with nothing in its place — an empty input would be a different run
under the same name, and copying the bytes from anywhere else would be
undoing an erasure somebody was owed.
*/
func (s *Server) recordedInput(ctx context.Context, source domain.Step) ([]byte, rerunRefusal, error) {
	var payload domain.RunStartedPayload
	decodePayload(source.Payload, &payload)
	if payload.InputRef == "" {
		// Opened with nothing: a schedule, a button pressed without text.
		// Running again with nothing is exactly what was recorded.
		return nil, nil, nil
	}
	if s.content == nil {
		return nil, nil, fmt.Errorf("no content store to read %s", source.RunID)
	}
	data, err := s.content.Get(ctx, payload.InputRef)
	if errors.Is(err, domain.ErrContentErased) || errors.Is(err, domain.ErrContentAbsent) {
		return nil, openapi.RunFromInput409ApplicationProblemPlusJSONResponse(
			refusal(http.StatusConflict, CodeSourceInputErased, "Source input erased", string(source.RunID))), nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read input of %s: %w", source.RunID, err)
	}
	return data, nil, nil
}

// openRefusal is a decision on this side not to open, said as one.
func openRefusal(err error) rerunRefusal {
	if errors.Is(err, trigger.ErrUnknownVersion) {
		return openapi.RunFromInput400ApplicationProblemPlusJSONResponse(invalid(err.Error()))
	}
	if notRunning(err) {
		return openapi.RunFromInput409ApplicationProblemPlusJSONResponse(conflicted(err.Error()))
	}
	return nil
}

func (s *Server) rerunAnswer(ctx context.Context, opened trigger.Result) (openapi.RunFromInputResponseObject, error) {
	run, _, err := s.project(ctx, opened.RunID)
	if err != nil {
		return nil, fmt.Errorf("project %s: %w", opened.RunID, err)
	}
	if opened.Created {
		return openapi.RunFromInput201JSONResponse(run), nil
	}
	return openapi.RunFromInput200JSONResponse(run), nil
}

func versionAsked(body *openapi.RunFromInputJSONRequestBody) domain.VersionID {
	if body == nil || body.VersionId == nil {
		return ""
	}
	return domain.VersionID(*body.VersionId)
}

// sourceRunOf is the recorded run a run was opened from, read off its opening
// step. Nothing else of the source is read: the id is the whole reference.
func sourceRunOf(steps []domain.Step) *string {
	if len(steps) == 0 || steps[0].Kind != domain.StepRunStarted {
		return nil
	}
	var payload struct {
		SourceRun domain.RunID `json:"source_run"`
	}
	if json.Unmarshal(steps[0].Payload, &payload) != nil || payload.SourceRun == "" {
		return nil
	}
	return ptr(string(payload.SourceRun))
}
