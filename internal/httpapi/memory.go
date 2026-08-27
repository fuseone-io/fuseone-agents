package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	memstore "github.com/fuseone/agents/internal/memory"
)

type Memory interface {
	List(ctx context.Context, f memstore.Filter) ([]domain.MemoryAssertion, error)
	Assert(ctx context.Context, a domain.MemoryAssertion, by domain.UserID, reason string,
		now time.Time) (domain.MemoryAssertion, error)
	Disable(ctx context.Context, id string, scope domain.Scope, by domain.UserID,
		reason string, now time.Time) error
	ListSuggestions(ctx context.Context, f memstore.SuggestionFilter) ([]domain.MemorySuggestion, error)
	AcceptSuggestion(ctx context.Context, id string, scope domain.Scope, by domain.UserID,
		reason string, now time.Time) (domain.MemoryAssertion, error)
	DismissSuggestion(ctx context.Context, id string, scope domain.Scope, by domain.UserID,
		reason string, now time.Time) error
}

func (s *Server) WithMemory(memory Memory) *Server {
	s.memory = memory
	return s
}

func (s *Server) ListMemoryAssertions(
	ctx context.Context, req openapi.ListMemoryAssertionsRequestObject,
) (openapi.ListMemoryAssertionsResponseObject, error) {
	scopes, refused := memoryReadableScopes(ctx, req.Params)
	if refused != nil {
		return openapi.ListMemoryAssertions403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	if s.memory == nil {
		return openapi.ListMemoryAssertions200JSONResponse{Items: []openapi.MemoryAssertion{}}, nil
	}
	items, err := s.memory.List(ctx, memoryFilter(scopes, req.Params))
	if err != nil {
		return nil, fmt.Errorf("list memory assertions: %w", err)
	}
	return openapi.ListMemoryAssertions200JSONResponse{Items: memoryAssertions(items)}, nil
}

func (s *Server) CreateMemoryAssertion(
	ctx context.Context, req openapi.CreateMemoryAssertionRequestObject,
) (openapi.CreateMemoryAssertionResponseObject, error) {
	if s.memory == nil || req.Body == nil {
		return badMemoryCreate("memory assertion body is required"), nil
	}
	scope, err := inputScope(req.Body.Company, req.Body.Area)
	if err != nil {
		return badMemoryCreate(err.Error()), nil
	}
	if err := auth.Require(ctx, domain.PermAgentPublish, scope); err != nil {
		return forbiddenMemoryCreate(domain.PermAgentPublish, scope), nil
	}
	labels, err := s.labelsFromMemoryEvidence(ctx, scope, req.Body.Evidence)
	if err != nil {
		return badMemoryCreate(err.Error()), nil
	}
	assertion, err := s.memory.Assert(ctx, memoryAssertionInput(*req.Body, scope, labels),
		callerOf(ctx), req.Body.Reason, clockOr(s.clock).Now())
	switch memoryRefusal(err) {
	case http.StatusConflict:
		return openapi.CreateMemoryAssertion409ApplicationProblemPlusJSONResponse(
			conflicted(err.Error())), nil
	case http.StatusBadRequest:
		return badMemoryCreate(err.Error()), nil
	}
	if err != nil {
		return nil, fmt.Errorf("create memory assertion: %w", err)
	}
	return openapi.CreateMemoryAssertion200JSONResponse(memoryAssertion(assertion)), nil
}

func (s *Server) DisableMemoryAssertion(
	ctx context.Context, req openapi.DisableMemoryAssertionRequestObject,
) (openapi.DisableMemoryAssertionResponseObject, error) {
	if s.memory == nil || req.Body == nil {
		return badMemoryDisable("memory disable body is required"), nil
	}
	scope, err := inputScope(req.Body.Company, req.Body.Area)
	if err != nil {
		return badMemoryDisable(err.Error()), nil
	}
	if err := auth.Require(ctx, domain.PermAgentPublish, scope); err != nil {
		return forbiddenMemoryDisable(domain.PermAgentPublish, scope), nil
	}
	err = s.memory.Disable(ctx, req.AssertionId, scope, callerOf(ctx), req.Body.Reason, clockOr(s.clock).Now())
	if errors.Is(err, memstore.ErrNotFound) {
		return openapi.DisableMemoryAssertion404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.AssertionId),
		}, nil
	}
	if memoryRefusal(err) == http.StatusBadRequest {
		return badMemoryDisable(err.Error()), nil
	}
	if err != nil {
		return nil, fmt.Errorf("disable memory assertion %s: %w", req.AssertionId, err)
	}
	return openapi.DisableMemoryAssertion204Response{}, nil
}

func (s *Server) ListMemorySuggestions(
	ctx context.Context, req openapi.ListMemorySuggestionsRequestObject,
) (openapi.ListMemorySuggestionsResponseObject, error) {
	scopes, refused := suggestionReadableScopes(ctx, req.Params)
	if refused != nil {
		return openapi.ListMemorySuggestions403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	if s.memory == nil {
		return openapi.ListMemorySuggestions200JSONResponse{Items: []openapi.MemorySuggestion{}}, nil
	}
	items, err := s.memory.ListSuggestions(ctx, suggestionFilter(scopes, req.Params))
	if err != nil {
		return nil, fmt.Errorf("list memory suggestions: %w", err)
	}
	return openapi.ListMemorySuggestions200JSONResponse{Items: memorySuggestions(items)}, nil
}

func (s *Server) AcceptMemorySuggestion(
	ctx context.Context, req openapi.AcceptMemorySuggestionRequestObject,
) (openapi.AcceptMemorySuggestionResponseObject, error) {
	if s.memory == nil || req.Body == nil {
		return badMemorySuggestionAccept("memory suggestion review body is required"), nil
	}
	scope, err := inputScope(req.Body.Company, req.Body.Area)
	if err != nil {
		return badMemorySuggestionAccept(err.Error()), nil
	}
	if err := auth.Require(ctx, domain.PermAgentPublish, scope); err != nil {
		return forbiddenMemorySuggestionAccept(domain.PermAgentPublish, scope), nil
	}
	assertion, err := s.memory.AcceptSuggestion(ctx, req.SuggestionId, scope,
		callerOf(ctx), req.Body.Reason, clockOr(s.clock).Now())
	if errors.Is(err, memstore.ErrNotFound) {
		return openapi.AcceptMemorySuggestion404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.SuggestionId),
		}, nil
	}
	switch memoryRefusal(err) {
	case http.StatusConflict:
		return openapi.AcceptMemorySuggestion409ApplicationProblemPlusJSONResponse(
			conflicted(err.Error())), nil
	case http.StatusBadRequest:
		return badMemorySuggestionAccept(err.Error()), nil
	}
	if err != nil {
		return nil, fmt.Errorf("accept memory suggestion %s: %w", req.SuggestionId, err)
	}
	return openapi.AcceptMemorySuggestion200JSONResponse(memoryAssertion(assertion)), nil
}

func (s *Server) DismissMemorySuggestion(
	ctx context.Context, req openapi.DismissMemorySuggestionRequestObject,
) (openapi.DismissMemorySuggestionResponseObject, error) {
	if s.memory == nil || req.Body == nil {
		return badMemorySuggestionDismiss("memory suggestion review body is required"), nil
	}
	scope, err := inputScope(req.Body.Company, req.Body.Area)
	if err != nil {
		return badMemorySuggestionDismiss(err.Error()), nil
	}
	if err := auth.Require(ctx, domain.PermAgentPublish, scope); err != nil {
		return forbiddenMemorySuggestionDismiss(domain.PermAgentPublish, scope), nil
	}
	err = s.memory.DismissSuggestion(ctx, req.SuggestionId, scope,
		callerOf(ctx), req.Body.Reason, clockOr(s.clock).Now())
	if errors.Is(err, memstore.ErrNotFound) {
		return openapi.DismissMemorySuggestion404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.SuggestionId),
		}, nil
	}
	if memoryRefusal(err) == http.StatusBadRequest {
		return badMemorySuggestionDismiss(err.Error()), nil
	}
	if err != nil {
		return nil, fmt.Errorf("dismiss memory suggestion %s: %w", req.SuggestionId, err)
	}
	return openapi.DismissMemorySuggestion204Response{}, nil
}

func memoryReadableScopes(
	ctx context.Context, params openapi.ListMemoryAssertionsParams,
) ([]domain.Scope, *openapi.ForbiddenApplicationProblemPlusJSONResponse) {
	requested := memoryScopeParam(params.Company, params.Area)
	if requested.Company != "" || requested.Area != "" {
		if err := auth.Require(ctx, domain.PermAgentRead, requested); err != nil {
			body := forbidden(domain.PermAgentRead, requested)
			return nil, &body
		}
		return []domain.Scope{requested}, nil
	}
	visible := auth.VisibleScopes(ctx, domain.PermAgentRead)
	if len(visible) == 0 {
		body := forbidden(domain.PermAgentRead, domain.Scope{})
		return nil, &body
	}
	return visible, nil
}

func memoryFilter(scopes []domain.Scope, params openapi.ListMemoryAssertionsParams) memstore.Filter {
	filter := memstore.Filter{Scopes: scopes, Limit: limitOf(params.Limit)}
	if params.AgentId != nil {
		filter.AgentID = domain.AgentID(strings.TrimSpace(*params.AgentId))
	}
	if params.Status != nil {
		filter.Status = domain.MemoryStatus(*params.Status)
	}
	if params.Q != nil {
		filter.Search = *params.Q
	}
	return filter
}

func suggestionReadableScopes(
	ctx context.Context, params openapi.ListMemorySuggestionsParams,
) ([]domain.Scope, *openapi.ForbiddenApplicationProblemPlusJSONResponse) {
	requested := memoryScopeParam(params.Company, params.Area)
	if requested.Company != "" || requested.Area != "" {
		if err := auth.Require(ctx, domain.PermAgentRead, requested); err != nil {
			body := forbidden(domain.PermAgentRead, requested)
			return nil, &body
		}
		return []domain.Scope{requested}, nil
	}
	visible := auth.VisibleScopes(ctx, domain.PermAgentRead)
	if len(visible) == 0 {
		body := forbidden(domain.PermAgentRead, domain.Scope{})
		return nil, &body
	}
	return visible, nil
}

func suggestionFilter(scopes []domain.Scope, params openapi.ListMemorySuggestionsParams) memstore.SuggestionFilter {
	filter := memstore.SuggestionFilter{Scopes: scopes, Limit: limitOf(params.Limit)}
	if params.AgentId != nil {
		filter.AgentID = domain.AgentID(strings.TrimSpace(*params.AgentId))
	}
	if params.Status != nil {
		filter.Status = domain.MemorySuggestionStatus(*params.Status)
	}
	if params.Q != nil {
		filter.Search = *params.Q
	}
	return filter
}

func memoryScopeParam(company *openapi.CompanyScope, area *openapi.Area) domain.Scope {
	var scope domain.Scope
	if company != nil {
		scope.Company = domain.CompanyID(*company)
	}
	if area != nil {
		scope.Area = domain.AreaID(*area)
	}
	return scope
}

func inputScope(company, area string) (domain.Scope, error) {
	scope := domain.Scope{Company: domain.CompanyID(strings.TrimSpace(company)), Area: domain.AreaID(strings.TrimSpace(area))}
	if !scope.Valid() || scope.Company == domain.Installation {
		return domain.Scope{}, fmt.Errorf("memory scope must name a company and area")
	}
	return scope, nil
}

func (s *Server) labelsFromMemoryEvidence(
	ctx context.Context, scope domain.Scope, evidence []openapi.MemoryEvidence,
) (domain.Labels, error) {
	var labels domain.Labels
	for _, ev := range evidence {
		seen, err := s.memoryEvidenceLabels(ctx, scope, ev)
		if err != nil {
			return nil, err
		}
		labels = labels.Union(seen)
	}
	return labels, nil
}

func (s *Server) memoryEvidenceLabels(
	ctx context.Context, scope domain.Scope, ev openapi.MemoryEvidence,
) (domain.Labels, error) {
	steps, err := s.store.Read(ctx, domain.RunID(ev.RunId), domain.FirstSeq)
	if err != nil || len(steps) == 0 || !scope.Contains(steps[0].Scope) {
		return nil, fmt.Errorf("memory evidence is outside scope or absent")
	}
	for _, step := range steps {
		if step.Kind != domain.StepRunFinished {
			continue
		}
		if labels, ok := finishedArtifactLabels(step, ev); ok {
			return labels, nil
		}
	}
	return nil, fmt.Errorf("memory evidence artifact does not match the ledger")
}

func finishedArtifactLabels(step domain.Step, ev openapi.MemoryEvidence) (domain.Labels, bool) {
	var p domain.RunFinishedPayload
	if err := json.Unmarshal(step.Payload, &p); err != nil {
		return nil, false
	}
	if ev.Artifact == domain.ArtifactFinalAnswer && ev.Digest == p.OutcomeDigest {
		return step.Labels.Clone(), true
	}
	for _, artifact := range p.Artifacts {
		if artifact.Name == ev.Artifact && artifact.Digest == ev.Digest {
			return artifact.Labels.Clone(), true
		}
	}
	return nil, false
}

func memoryAssertionInput(
	in openapi.MemoryAssertionInput, scope domain.Scope, labels domain.Labels,
) domain.MemoryAssertion {
	observations, confirmed := int64(1), int64(1)
	if in.Observations != nil {
		observations = *in.Observations
	}
	if in.Confirmed != nil {
		confirmed = *in.Confirmed
	}
	return domain.MemoryAssertion{
		Scope: scope, AgentID: domain.AgentID(valueOr(in.AgentId)),
		Kind: in.Kind, Subject: in.Subject, Signature: in.Signature, Claim: in.Claim,
		Evidence: memoryEvidenceFrom(in.Evidence), Observations: observations,
		Confirmed: confirmed, Labels: labels, Status: domain.MemoryActive,
		ExpiresAt: in.ExpiresAt,
	}
}

func memoryAssertions(in []domain.MemoryAssertion) []openapi.MemoryAssertion {
	out := make([]openapi.MemoryAssertion, 0, len(in))
	for _, a := range in {
		out = append(out, memoryAssertion(a))
	}
	return out
}

func memoryAssertion(a domain.MemoryAssertion) openapi.MemoryAssertion {
	return openapi.MemoryAssertion{
		Id: string(a.ID), Scope: openapi.Scope{
			Company: string(a.Scope.Company), Area: string(a.Scope.Area),
		},
		AgentId: string(a.AgentID), Kind: a.Kind, Subject: a.Subject,
		Signature: a.Signature, Claim: a.Claim,
		Evidence: memoryEvidenceTo(a.Evidence), Observations: a.Observations,
		Confirmed: a.Confirmed, Labels: []string(a.Labels),
		Status: openapi.MemoryStatus(a.Status), ExpiresAt: a.ExpiresAt,
		CreatedBy: string(a.CreatedBy), CreatedAt: a.CreatedAt,
		UpdatedBy: string(a.UpdatedBy), UpdatedAt: a.UpdatedAt,
	}
}

func memorySuggestions(in []domain.MemorySuggestion) []openapi.MemorySuggestion {
	out := make([]openapi.MemorySuggestion, 0, len(in))
	for _, s := range in {
		out = append(out, memorySuggestion(s))
	}
	return out
}

func memorySuggestion(s domain.MemorySuggestion) openapi.MemorySuggestion {
	return openapi.MemorySuggestion{
		Id: s.ID, AssertionId: s.AssertionID, Scope: openapi.Scope{
			Company: string(s.Scope.Company), Area: string(s.Scope.Area),
		},
		AgentId: string(s.AgentID), Kind: s.Kind, Subject: s.Subject,
		Signature: s.Signature, Claim: s.Claim,
		Evidence: memoryEvidenceTo(s.Evidence), Observations: s.Observations,
		Labels: []string(s.Labels), Status: openapi.MemorySuggestionStatus(s.Status),
		ExpiresAt: s.ExpiresAt, CreatedBy: string(s.CreatedBy), CreatedAt: s.CreatedAt,
		UpdatedBy: string(s.UpdatedBy), UpdatedAt: s.UpdatedAt,
	}
}

func memoryEvidenceFrom(in []openapi.MemoryEvidence) []domain.MemoryEvidence {
	out := make([]domain.MemoryEvidence, 0, len(in))
	for _, ev := range in {
		out = append(out, domain.MemoryEvidence{
			RunID: domain.RunID(ev.RunId), Artifact: ev.Artifact, Digest: ev.Digest,
		})
	}
	return out
}

func memoryEvidenceTo(in []domain.MemoryEvidence) []openapi.MemoryEvidence {
	out := make([]openapi.MemoryEvidence, 0, len(in))
	for _, ev := range in {
		out = append(out, openapi.MemoryEvidence{
			RunId: string(ev.RunID), Artifact: ev.Artifact, Digest: ev.Digest,
		})
	}
	return out
}

/*
memoryRefusal says which kind of no the store gave, or zero when it is not one.

Three answers used to be one. A body the server would not accept, a state that
contradicts the write, and a database that is not answering all left here as
400 with a sentence — so the console offered "check what you typed" to somebody
whose installation was down, and the same thing to somebody whose memory holds
two rows claiming one identity, which is the only one of the three a person can
go and fix.

Zero is deliberately the answer for an error this package does not recognise.
An unrecognised failure is the installation's, and it belongs in the logs as a
failure rather than on the screen as the caller's mistake.
*/
func memoryRefusal(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, memstore.ErrCanonicalConflict),
		errors.Is(err, memstore.ErrMemoryTerminal),
		errors.Is(err, memstore.ErrCovered),
		errors.Is(err, memstore.ErrEvidenceCannotExplain):
		return http.StatusConflict
	case errors.Is(err, memstore.ErrInvalid):
		return http.StatusBadRequest
	}
	return 0
}

func badMemoryCreate(detail string) openapi.CreateMemoryAssertion400ApplicationProblemPlusJSONResponse {
	return openapi.CreateMemoryAssertion400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			invalid(detail)),
	}
}

func forbiddenMemoryCreate(
	perm domain.Permission, scope domain.Scope,
) openapi.CreateMemoryAssertion403ApplicationProblemPlusJSONResponse {
	return openapi.CreateMemoryAssertion403ApplicationProblemPlusJSONResponse{
		ForbiddenApplicationProblemPlusJSONResponse: forbidden(perm, scope),
	}
}

func badMemoryDisable(detail string) openapi.DisableMemoryAssertion400ApplicationProblemPlusJSONResponse {
	return openapi.DisableMemoryAssertion400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			invalid(detail)),
	}
}

func forbiddenMemoryDisable(
	perm domain.Permission, scope domain.Scope,
) openapi.DisableMemoryAssertion403ApplicationProblemPlusJSONResponse {
	return openapi.DisableMemoryAssertion403ApplicationProblemPlusJSONResponse{
		ForbiddenApplicationProblemPlusJSONResponse: forbidden(perm, scope),
	}
}

func badMemorySuggestionAccept(detail string) openapi.AcceptMemorySuggestion400ApplicationProblemPlusJSONResponse {
	return openapi.AcceptMemorySuggestion400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			invalid(detail)),
	}
}

func forbiddenMemorySuggestionAccept(
	perm domain.Permission, scope domain.Scope,
) openapi.AcceptMemorySuggestion403ApplicationProblemPlusJSONResponse {
	return openapi.AcceptMemorySuggestion403ApplicationProblemPlusJSONResponse{
		ForbiddenApplicationProblemPlusJSONResponse: forbidden(perm, scope),
	}
}

func badMemorySuggestionDismiss(detail string) openapi.DismissMemorySuggestion400ApplicationProblemPlusJSONResponse {
	return openapi.DismissMemorySuggestion400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			invalid(detail)),
	}
}

func forbiddenMemorySuggestionDismiss(
	perm domain.Permission, scope domain.Scope,
) openapi.DismissMemorySuggestion403ApplicationProblemPlusJSONResponse {
	return openapi.DismissMemorySuggestion403ApplicationProblemPlusJSONResponse{
		ForbiddenApplicationProblemPlusJSONResponse: forbidden(perm, scope),
	}
}
