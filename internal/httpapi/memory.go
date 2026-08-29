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
	Match(ctx context.Context, in memstore.MatchInput) (memstore.Match, error)
	ListSuggestions(ctx context.Context, f memstore.SuggestionFilter) ([]domain.MemorySuggestion, error)
	AcceptSuggestion(ctx context.Context, in memstore.AcceptInput) (domain.MemoryAssertion, error)
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
	agent, labels, cited, err := s.originOfMemoryEvidence(ctx, scope, req.Body.Namespace, req.Body.Evidence)
	if evidenceRefused(err) {
		return badMemoryCreate(err.Error()), nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve memory evidence: %w", err)
	}
	now := clockOr(s.clock).Now()
	// The same policy the accept applies, from the same function, on the
	// assertion this is about to write rather than on the request that
	// described it.
	proposed, _, err := memoryAssertionInput(*req.Body, scope,
		memoryOrigin{agent: agent, labels: labels, cited: cited, now: now})
	if err != nil {
		return badMemoryCreate(err.Error()), nil
	}
	proposed, err = memstore.SecretDecision(proposed,
		overridesSecretWarning(*req.Body), req.Body.Reason)
	if refused := memorySecretProblem(err); refused != nil {
		return openapi.CreateMemoryAssertion400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(*refused),
		}, nil
	}
	assertion, err := s.memory.Assert(ctx, proposed, callerOf(ctx), req.Body.Reason, now)
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
/*
evidenceRefused is a citation the caller can fix, as opposed to a platform that
could not answer.

The same sentinels mean different things at different doors, which is why this
is not memoryRefusal. On a creation the citation is input: naming an artifact
the run never published, or one whose bytes are gone, is something the person
typed and can retype. On a reactivation the same sentinels describe stored state
somebody has to decide about, and that is a conflict.

Everything else — the ledger unreachable, the content store away, no resolver
wired at all — is the installation's, and telling somebody to check their form
about it is the oldest way to waste an afternoon.
*/
func evidenceRefused(err error) bool {
	return errors.Is(err, memstore.ErrInvalid) ||
		errors.Is(err, memstore.ErrEvidenceInvalid) ||
		errors.Is(err, memstore.ErrEvidenceSourceAbsent)
}

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
memorySecretProblem is how the store's two secret refusals reach a caller.

Codes rather than sentences, and neither carries the text that triggered it: an
error quoting the token would copy it into a log, an audit event and whatever
bug report somebody pastes it into, which is three more places than the one it
was already in.
*/
func memorySecretProblem(err error) *openapi.Problem {
	switch {
	case errors.Is(err, memstore.ErrSecret):
		p := refusal(http.StatusBadRequest, CodeMemorySecret, "Looks like a credential",
			"a private key or a complete token was recognised")
		return &p
	case errors.Is(err, memstore.ErrSecretSuspected):
		p := refusal(http.StatusBadRequest, CodeMemorySecretWarned, "May be a credential",
			"text long enough and random enough to be one")
		return &p
	}
	return nil
}

func overridesSecretWarning(in openapi.MemoryAssertionInput) bool {
	return in.OverrideSecretWarning != nil && *in.OverrideSecretWarning
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
