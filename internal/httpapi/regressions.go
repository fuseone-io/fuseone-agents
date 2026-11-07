package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Regressions is the corpus an agent is held to, declared here by the
// consumer.
type Regressions interface {
	Record(ctx context.Context, c domain.RegressionCase) error
	List(ctx context.Context, agent domain.AgentID) ([]domain.RegressionCase, error)
	Delete(ctx context.Context, agent domain.AgentID, id string, by domain.UserID) error
	// Hold fences the corpus against an erasure while it is being copied.
	Hold(ctx context.Context, agent domain.AgentID, inside func(context.Context) error) error
}

// WithRegressions wires the corrections a future version is checked against.
func (s *Server) WithRegressions(corpus Regressions) *Server {
	s.regressions = corpus
	return s
}

func (s *Server) ListRegressions(
	ctx context.Context, req openapi.ListRegressionsRequestObject,
) (openapi.ListRegressionsResponseObject, error) {
	absent := openapi.ListRegressions404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}
	published, ok, err := s.publishedAgent(ctx, req.AgentId)
	if err != nil || !ok {
		return absent, err
	}
	if err := auth.Require(ctx, domain.PermAgentRead, published.Scope); err != nil {
		return openapi.ListRegressions403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentRead, published.Scope),
		}, nil
	}
	if s.regressions == nil {
		return openapi.ListRegressions200JSONResponse{Items: []openapi.RegressionCase{}}, nil
	}

	corpus, err := s.regressions.List(ctx, published.ID)
	if err != nil {
		return nil, fmt.Errorf("list regressions: %w", err)
	}
	items := make([]openapi.RegressionCase, 0, len(corpus))
	for _, c := range corpus {
		items = append(items, toRegressionCase(c))
	}
	return openapi.ListRegressions200JSONResponse{Items: items}, nil
}

// RecordRegression turns a run that came out wrong into a case every future
// version is checked against.
func (s *Server) RecordRegression(
	ctx context.Context, req openapi.RecordRegressionRequestObject,
) (openapi.RecordRegressionResponseObject, error) {
	absent := openapi.RecordRegression404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}
	published, ok, err := s.publishedAgent(ctx, req.AgentId)
	if err != nil || !ok {
		return absent, err
	}
	// Correcting an agent is authoring it: what this writes decides whether a
	// future version may be published at all.
	if err := auth.Require(ctx, domain.PermAgentPublish, published.Scope); err != nil {
		return openapi.RecordRegression403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, published.Scope),
		}, nil
	}
	if s.regressions == nil || s.cases == nil || s.content == nil || req.Body == nil {
		return absent, nil
	}

	recorded, refused, err := s.recordCorrection(ctx, published, req.Body)
	if err != nil {
		return nil, err
	}
	if refused != nil {
		return openapi.RecordRegression400ApplicationProblemPlusJSONResponse(
			invalid(refused.Error())), nil
	}
	return openapi.RecordRegression201JSONResponse(toRegressionCase(recorded)), nil
}

/*
recordCorrection copies what a run was about into the corpus.

Reading the run and writing the copy happen inside the fence an erasure takes. An
erasure between them would be undone by the write, and would report success.
*/
func (s *Server) recordCorrection(
	ctx context.Context, published domain.AgentSummary, body *openapi.RecordRegressionJSONRequestBody,
) (recorded domain.RegressionCase, refused, err error) {
	err = s.regressions.Hold(ctx, published.ID, func(ctx context.Context) error {
		input, readErr := s.occurrenceOf(ctx, published, domain.RunID(body.RunId))
		if readErr != nil {
			refused = readErr
			return nil
		}
		// Copied into the corpus rather than pointed at inside the run, so a case
		// outlives the purge of the run it came from. The copy is under retention
		// itself, and recording a case again after it aged out has to work: the
		// reference is the digest, so this lands on the old copy's tombstone.
		ref, putErr := s.cases.RestoreFor(ctx, "regression", string(published.ID), 1, input)
		if putErr != nil {
			return fmt.Errorf("keep the occurrence: %w", putErr)
		}
		recorded = domain.RegressionCase{
			ID:           regressionID(published.ID, body.RunId),
			Agent:        published.ID,
			Scope:        published.Scope,
			InputRef:     ref,
			Expectations: fromExpectations(body.Expectations),
			FromRun:      domain.RunID(body.RunId),
			CreatedBy:    callerOf(ctx),
		}
		if body.Note != nil {
			recorded.Note = *body.Note
		}
		refused = s.regressions.Record(ctx, recorded)
		return nil
	})
	return recorded, refused, err
}

func (s *Server) DeleteRegression(
	ctx context.Context, req openapi.DeleteRegressionRequestObject,
) (openapi.DeleteRegressionResponseObject, error) {
	published, ok, err := s.publishedAgent(ctx, req.AgentId)
	if err != nil || !ok {
		return openapi.DeleteRegression204Response{}, err
	}
	if err := auth.Require(ctx, domain.PermAgentPublish, published.Scope); err != nil {
		return openapi.DeleteRegression403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, published.Scope),
		}, nil
	}
	if s.regressions == nil {
		return openapi.DeleteRegression204Response{}, nil
	}

	if err := s.regressions.Delete(ctx, published.ID, req.CaseId, callerOf(ctx)); err != nil {
		return nil, fmt.Errorf("delete regression %s: %w", req.CaseId, err)
	}
	return openapi.DeleteRegression204Response{}, nil
}

// occurrenceOf reads back what one of this agent's runs was about, in the scope it
// is published in.
//
// Authority was checked on that scope and the copy goes there, so a run of another
// agent, or an old run of this one under another scope, would carry content across
// the boundary. Both read as absent, which says nothing about whether they exist.
// The read is checked as well, because this hands a person a run's input.
func (s *Server) occurrenceOf(
	ctx context.Context, published domain.AgentSummary, runID domain.RunID,
) ([]byte, error) {
	absent := fmt.Errorf("no such run: %s", runID)
	steps, err := s.store.Read(ctx, runID, domain.FirstSeq)
	if err != nil || len(steps) == 0 {
		return nil, absent
	}
	if steps[0].AgentID != published.ID || steps[0].Scope != published.Scope {
		return nil, absent
	}
	if err := auth.Require(ctx, domain.PermRunRead, steps[0].Scope); err != nil {
		return nil, absent
	}

	var started domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
		return nil, fmt.Errorf("read what %s was about: %w", runID, err)
	}
	if started.InputRef == "" {
		// A run opened with nothing cannot be replayed, and a corpus case with
		// no occurrence would be an expectation about nothing.
		return nil, fmt.Errorf("run %s was opened with no input", runID)
	}
	return s.content.Get(ctx, started.InputRef)
}

// regressionID names a correction after the run it was made from, so making
// the same correction twice from the same run replaces it rather than filling
// the corpus with duplicates of one complaint.
func regressionID(agent domain.AgentID, run string) string {
	sum := sha256.Sum256([]byte(run))
	return fmt.Sprintf("reg_%s_%s", agent, hex.EncodeToString(sum[:6]))
}
