package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	memstore "github.com/fuseone/agents/internal/memory"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// The memory a person records, corrects and turns off.
//
// The proposals an agent makes are next door: a suggestion is not a memory
// until somebody agrees, and the two have different permissions to check and
// different endings to write.

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
	Reactivate(ctx context.Context, r *memstore.Resolver,
		in memstore.ReactivateInput) (domain.MemoryAssertion, error)
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
	// Checked here because nothing else does. The generated strict handler
	// decodes the body and validates neither required fields nor enums, so a
	// namespace left out arrives as the empty string and an unknown one arrives
	// as itself — and both used to be read as the narrow value, which is the
	// safe direction for one of those mistakes and luck for the other.
	//
	// The generated Valid rather than a copy of the two values: the enum is the
	// contract's, and a second list here would be the thing that still says two
	// when the spec says three.
	if !req.Body.Namespace.Valid() {
		return badMemoryCreate("namespace must be agent or shared"), nil
	}
	if refused := memorySecretRefusal(*req.Body); refused != nil {
		return openapi.CreateMemoryAssertion400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(*refused),
		}, nil
	}
	agent, labels, err := s.originOfMemoryEvidence(ctx, scope, req.Body.Namespace, req.Body.Evidence)
	if err != nil {
		return badMemoryCreate(err.Error()), nil
	}
	now := clockOr(s.clock).Now()
	assertion, err := s.memory.Assert(ctx,
		memoryAssertionInput(*req.Body, scope, memoryOrigin{agent: agent, labels: labels, now: now}),
		callerOf(ctx), req.Body.Reason, now)
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
		errors.Is(err, memstore.ErrEvidenceCannotExplain),
		errors.Is(err, memstore.ErrEvidenceInvalid),
		errors.Is(err, memstore.ErrMovedMeanwhile):
		return http.StatusConflict
	case errors.Is(err, memstore.ErrInvalid):
		return http.StatusBadRequest
	}
	return 0
}

/*
memorySecretRefusal answers what the text somebody typed looks like.

The fields a model can read, and only those: a memory is quoted back into a run,
so a credential in one of them is a credential the platform hands to a model.
The evidence is left out because nothing a person composes goes in it — but that
is a narrowing, not a protection. What keeps a digest from being read as a
credential is the rule itself, which is blind to hexadecimal, and that is where
it is tested.

The detail names the risk and never the text. An error quoting the token would
copy it into a log, an audit event and whatever bug report somebody pastes it
into, which is three more places than the one it was already in.

Certain is refused outright and no acknowledgement clears it. A warning can be
answered, once a person has actually been shown it: the flag exists so the
console can carry that answer back, not so a client can pre-emptively opt out of
being asked.
*/
func memorySecretRefusal(in openapi.MemoryAssertionInput) *openapi.Problem {
	switch domain.LooksLikeSecret(in.Kind, in.Subject, in.Signature, in.Claim, in.Reason) {
	case domain.SecretCertain:
		p := refusal(http.StatusBadRequest, CodeMemorySecret, "Looks like a credential",
			"a private key or a complete token was recognised")
		return &p
	case domain.SecretSuspected:
		if in.AcknowledgedSecretWarning != nil && *in.AcknowledgedSecretWarning {
			return nil
		}
		p := refusal(http.StatusBadRequest, CodeMemorySecretWarned, "May be a credential",
			"text long enough and random enough to be one")
		return &p
	}
	return nil
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
