package httpapi

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Content holds what the ledger records a reference to instead of the bytes.
//
// Declared here, by the consumer. The API reads content constantly and writes
// exactly one thing: the input a run is opened with, which originates at this
// edge and nowhere else. Everything a run produces afterwards is written by
// the worker that produced it.
type Content interface {
	Get(ctx context.Context, ref string) ([]byte, error)
	Put(ctx context.Context, runID domain.RunID, seq int64, data []byte) (string, error)
}

// WithContent wires the store the trail's references point into.
func (s *Server) WithContent(content Content) *Server {
	s.content = content
	return s
}

// GetStepContent resolves what a step references.
//
// Every failure answers the same way — no such run, no such step, a step that
// references nothing, content that has aged out of retention. The caller is
// asking whether some content is readable, and every negative answer to that
// is "no". Distinguishing them would tell somebody outside the area that a
// step exists and what kind it is.
func (s *Server) GetStepContent(
	ctx context.Context, req openapi.GetStepContentRequestObject,
) (openapi.GetStepContentResponseObject, error) {
	absent := openapi.GetStepContent404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
	}

	if s.content == nil {
		return absent, nil
	}
	steps, err := s.store.Read(ctx, domain.RunID(req.RunId), domain.FirstSeq)
	if err != nil {
		if isNotFound(err) {
			return absent, nil
		}
		return nil, fmt.Errorf("read steps: %w", err)
	}
	if len(steps) == 0 || !mayRead(ctx, domain.PermRunRead, steps[0].Scope) {
		return absent, nil
	}

	ref, digest := "", ""
	for _, step := range steps {
		if step.Seq == req.Seq {
			ref, digest = referenceOf(step)
			break
		}
	}
	if ref == "" {
		return absent, nil
	}

	data, err := s.content.Get(ctx, ref)
	if err != nil {
		// Content outlives nothing: retention deletes it while the step that
		// references it stays in the chain forever. A gone reference is a
		// normal state of an old run, not a server fault.
		return absent, nil
	}
	return openapi.GetStepContent200JSONResponse{
		Seq: req.Seq, Digest: digest, Content: string(data),
	}, nil
}

// referenceOf returns the content a step points at, and the digest recorded
// with it. A step carries at most one such reference.
func referenceOf(step domain.Step) (ref, digest string) {
	switch step.Kind {
	case domain.StepRunStarted:
		var p domain.RunStartedPayload
		decodePayload(step.Payload, &p)
		return p.InputRef, ""
	case domain.StepToolCalled:
		var p domain.ToolCalledPayload
		decodePayload(step.Payload, &p)
		return p.ArgsRef, p.ArgsDigest
	case domain.StepApprovalRequested:
		var p domain.ApprovalRequestedPayload
		decodePayload(step.Payload, &p)
		return p.ArgsRef, p.ArgsDigest
	case domain.StepToolReturned:
		var p domain.ToolReturnedPayload
		decodePayload(step.Payload, &p)
		return p.ResultRef, ""
	case domain.StepRunFinished:
		var p domain.RunFinishedPayload
		decodePayload(step.Payload, &p)
		return p.OutcomeRef, p.OutcomeDigest
	}
	return "", ""
}

// decodePayload leaves the target zeroed on malformed JSON, which reads as
// "references nothing" — the same answer as a step that genuinely does not.
func decodePayload(raw []byte, into any) {
	_ = json.Unmarshal(raw, into)
}
