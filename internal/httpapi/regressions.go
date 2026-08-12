package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Regressions is the corpus an agent is held to, declared here by the
// consumer.
type Regressions interface {
	Record(ctx context.Context, c domain.RegressionCase) error
	List(ctx context.Context, agent domain.AgentID) ([]domain.RegressionCase, error)
	Delete(ctx context.Context, agent domain.AgentID, id string) error
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

	input, err := s.occurrenceOf(ctx, domain.RunID(req.Body.RunId))
	if err != nil {
		return openapi.RecordRegression400ApplicationProblemPlusJSONResponse(
			problem(http.StatusBadRequest, "The run has no occurrence to keep", err.Error())), nil
	}

	// Copied into the corpus rather than pointed at inside the run: runs are
	// purged on the installation's retention, and a corpus that lost its cases
	// would stop checking while still reporting green.
	ref, err := s.cases.PutFor(ctx, "regression", string(published.ID), 1, input)
	if err != nil {
		return nil, fmt.Errorf("keep the occurrence: %w", err)
	}

	recorded := domain.RegressionCase{
		ID:           regressionID(published.ID, req.Body.RunId),
		Agent:        published.ID,
		Scope:        published.Scope,
		InputRef:     ref,
		Expectations: fromExpectations(req.Body.Expectations),
		FromRun:      domain.RunID(req.Body.RunId),
		CreatedBy:    callerOf(ctx),
	}
	if req.Body.Note != nil {
		recorded.Note = *req.Body.Note
	}

	if err := s.regressions.Record(ctx, recorded); err != nil {
		return openapi.RecordRegression400ApplicationProblemPlusJSONResponse(
			problem(http.StatusBadRequest, "Correction refused", err.Error())), nil
	}
	return openapi.RecordRegression201JSONResponse(toRegressionCase(recorded)), nil
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

	if err := s.regressions.Delete(ctx, published.ID, req.CaseId); err != nil {
		return nil, fmt.Errorf("delete regression %s: %w", req.CaseId, err)
	}
	return openapi.DeleteRegression204Response{}, nil
}

// occurrenceOf reads back what a run was about.
func (s *Server) occurrenceOf(ctx context.Context, runID domain.RunID) ([]byte, error) {
	steps, err := s.store.Read(ctx, runID, domain.FirstSeq)
	if err != nil || len(steps) == 0 {
		return nil, fmt.Errorf("no such run: %s", runID)
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
