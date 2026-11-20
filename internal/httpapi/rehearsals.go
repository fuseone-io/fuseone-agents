package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
)

/*
Opening a live rehearsal (NT-018 phase 4, release 4b).

A rehearsal is one simulated run of a candidate on a case its author typed. Nothing in it
reaches a tool layer: the engine parks each call for the author to answer for the world.
What this route decides is only whether the run opens, and on what: the seal the caller may
publish in, the case as text (its bytes judged before decoding, RefuseRewrittenText), and,
inside the agent's exclusive fence, the key and the seal's bytes. The opener then refuses a
paused or stopped agent and only after that resolves the caller's own conversation on the
draft the seal came from.
*/

// maxRehearsalCaseBytes bounds the case an author types. A case is a message, not a
// dataset: an upload belongs in a simulation.
const maxRehearsalCaseBytes = 32 << 10

type rehearsalAnswer = openapi.OpenRehearsalResponseObject

// rehearsalAsk is what a request under a key asks for, compared on a retry.
type rehearsalAsk struct {
	seal, caseID string
}

/*
OpenRehearsal opens a rehearsal, or answers with the one its key already opened.

Before this runs, the transport has already refused a body that is not UTF-8 or escapes an
unpaired surrogate (RefuseRewrittenText), before any seal is looked up. Once the body is
decoded, the order is the experiment's: the seal under the caller's authority first (404 for
a seal the caller may not publish in, as for one that does not exist), then the case's 400,
then, inside the agent's fence, the key, before the seal's bytes are judged again, so a retry
after a lost answer reaches its run even once the seal is erased.
*/
func (s *Server) OpenRehearsal(ctx context.Context, req openapi.OpenRehearsalRequestObject) (rehearsalAnswer, error) {
	agent := domain.AgentID(req.AgentId)
	seal, refused, err := s.rehearsedSeal(ctx, agent, req.Body)
	if refused != nil || err != nil {
		return refused, err
	}
	if why := caseRefusal(req.Body.Case); why != "" {
		return openapi.OpenRehearsal400ApplicationProblemPlusJSONResponse(invalid(why)), nil
	}
	caller := callerOf(ctx)
	key := trigger.RehearsalKey(agent, caller, req.Params.IdempotencyKey)
	asked := rehearsalAsk{seal: seal.SealID, caseID: rehearsalCase(req.Body.Case)}
	return s.openRehearsal(ctx, asked, trigger.Request{
		Agent: agent, IdemKey: key, Trigger: "rehearsal", By: caller,
		Input: []byte(req.Body.Case), Labels: domain.NewLabels(domain.LabelUntrusted),
		Simulation: rehearsalSimulation(agent, key), Case: asked.caseID,
		SealID: seal.SealID, Rehearsal: true,
	})
}

// rehearsedSeal is the seal the request names, found under the caller's authority.
func (s *Server) rehearsedSeal(
	ctx context.Context, agent domain.AgentID, body *openapi.RehearsalRequest,
) (spec.CandidateSeal, rehearsalAnswer, error) {
	absent := openapi.OpenRehearsal404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(string(agent)),
	}
	if s.candidates == nil || s.drafts == nil || body == nil {
		return spec.CandidateSeal{}, absent, nil
	}
	seal, err := s.candidates.Seal(ctx, agent, body.SealId, publishScopes(ctx))
	if errors.Is(err, spec.ErrCandidateNotFound) {
		return spec.CandidateSeal{}, absent, nil
	}
	return seal, nil, err
}

/*
openRehearsal opens the run inside the agent's exclusive fence, the one an erasure of the
seal or of the author's conversation takes.

Everything that decides or writes happens inside it, in this order: the key, the seal's
bytes, then the opener, which refuses a paused or stopped agent before it resolves the
author's conversation, the first write. So a refused rehearsal writes nothing; an erasure
either comes first, and the rehearsal finds its tombstone or joins a conversation already
erased, or waits, and then reaches the run; and two attempts under one key cannot both store
a case, since the second finds the first's run.
*/
func (s *Server) openRehearsal(ctx context.Context, asked rehearsalAsk, req trigger.Request) (rehearsalAnswer, error) {
	var opened trigger.Result
	var refused rehearsalAnswer
	err := s.withinAgentFence(ctx, req.Agent, func(ctx context.Context) error {
		var err error
		opened, refused, err = s.openFenced(ctx, req)
		return err
	})
	switch {
	case refused != nil:
		return refused, nil
	case notRunning(err):
		return openapi.OpenRehearsal409ApplicationProblemPlusJSONResponse(conflicted(err.Error())), nil
	case errors.Is(err, trigger.ErrUnknownVersion):
		return openapi.OpenRehearsal404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(string(req.Agent)),
		}, nil
	case err != nil:
		return nil, fmt.Errorf("open a rehearsal of %s: %w", req.Agent, err)
	case !opened.Created:
		// The key already named a run: it is the answer only if it asked for the same thing.
		return s.rehearsalReplay(ctx, opened.RunID, asked)
	}
	return s.rehearsalRun(ctx, opened.RunID, true)
}

// openFenced is the fenced part of opening: the key, the seal's bytes, then the opener.
func (s *Server) openFenced(ctx context.Context, req trigger.Request) (trigger.Result, rehearsalAnswer, error) {
	existing, err := s.store.RunByIdemKey(ctx, req.IdemKey)
	switch {
	case err == nil:
		return trigger.Result{RunID: existing}, nil, nil
	case !errors.Is(err, domain.ErrRunNotFound):
		return trigger.Result{}, nil, fmt.Errorf("rehearsal by key: %w", err)
	}
	_, err = s.candidates.Sealed(ctx, req.Agent, req.SealID)
	if errors.Is(err, domain.ErrCandidateUnreadable) || errors.Is(err, spec.ErrCandidateNotFound) {
		return trigger.Result{}, openapi.OpenRehearsal409ApplicationProblemPlusJSONResponse(refusal(
			http.StatusConflict, CodeCandidateUnreadable, "Candidate unreadable", req.SealID)), nil
	}
	if err != nil {
		return trigger.Result{}, nil, err
	}
	opened, err := s.opener().Open(ctx, req)
	return opened, nil, err
}

// withinAgentFence runs open inside the agent's exclusive fence.
func (s *Server) withinAgentFence(ctx context.Context, agent domain.AgentID, open func(context.Context) error) error {
	if s.regressions == nil {
		return open(ctx)
	}
	return s.regressions.Exclusive(ctx, []domain.AgentID{agent}, open)
}

/*
rehearsalReplay answers a key that already names a run.

The run must be a rehearsal the caller may still publish in (404 otherwise, as for a key
never spent), of the same seal and the same case (409 otherwise). Judged from the run's
opening step alone, so an erased seal or case does not change the answer.
*/
func (s *Server) rehearsalReplay(ctx context.Context, id domain.RunID, asked rehearsalAsk) (rehearsalAnswer, error) {
	steps, err := s.store.Read(ctx, id, domain.FirstSeq)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", id, err)
	}
	var started domain.RunStartedPayload
	if len(steps) > 0 {
		decodePayload(steps[0].Payload, &started)
	}
	if len(steps) == 0 || !started.Rehearsal || !mayRead(ctx, domain.PermAgentPublish, steps[0].Scope) {
		return openapi.OpenRehearsal404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(string(id)),
		}, nil
	}
	if started.SealID != asked.seal || started.Case != asked.caseID {
		return openapi.OpenRehearsal409ApplicationProblemPlusJSONResponse(
			conflicted("this key already opened a rehearsal of another seal or case")), nil
	}
	return s.rehearsalRun(ctx, id, false)
}

func (s *Server) rehearsalRun(ctx context.Context, id domain.RunID, created bool) (rehearsalAnswer, error) {
	run, _, err := s.project(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("project %s: %w", id, err)
	}
	if created {
		return openapi.OpenRehearsal201JSONResponse(run), nil
	}
	return openapi.OpenRehearsal200JSONResponse(run), nil
}

// caseRefusal is why a typed case cannot be rehearsed, or empty when it can.
func caseRefusal(text string) string {
	switch {
	case strings.TrimSpace(text) == "":
		return "a rehearsal needs a case"
	case len(text) > maxRehearsalCaseBytes:
		return fmt.Sprintf("a case is at most %d bytes", maxRehearsalCaseBytes)
	case !utf8.ValidString(text):
		return "a case is UTF-8 text"
	}
	return ""
}

// rehearsalCase is the case's id in the ledger: a digest, so the immutable row holds no
// typed text, and a retry with other text is told apart from the same one.
func rehearsalCase(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "case_" + hex.EncodeToString(sum[:16])
}

// rehearsalSimulation names the rehearsal's simulation after its intention.
func rehearsalSimulation(agent domain.AgentID, key string) string {
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("reh_%s_%s", agent, hex.EncodeToString(sum[:8]))
}
