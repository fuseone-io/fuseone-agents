package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if err != nil {
		return badMemoryCreate(err.Error()), nil
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
	if err != nil {
		return badMemoryDisable(err.Error()), nil
	}
	return openapi.DisableMemoryAssertion204Response{}, nil
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
