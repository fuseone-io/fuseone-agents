package httpapi

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/model"
)

// Tokenisers builds a token counter for a configured provider. Declared here
// by the consumer, so the httpapi does not depend on how a provider is
// reached.
type Tokenisers interface {
	// Counter is asked at the agent's scope (NT-019).
	Counter(provider string, at domain.Scope, cfg model.Config) (model.Counter, error)
}

// WithTokenisers wires where an instruction's size is asked.
func (s *Server) WithTokenisers(t Tokenisers) *Server {
	s.tokenisers = t
	return s
}

// longestInstruction is the most text this will relay to a provider.
//
// Well beyond any instruction anybody writes, and there so that the endpoint
// cannot be used to push a payload of arbitrary size through the
// installation's credential.
const longestInstruction = 200_000

// CountInstructionTokens answers how large an instruction is to the model that
// will read it.
//
// Held by whoever may publish: the call reaches a configured provider with the
// installation's credential, and reading what an instruction costs is part of
// writing one rather than part of reading runs.
func (s *Server) CountInstructionTokens(
	ctx context.Context, req openapi.CountInstructionTokensRequestObject,
) (openapi.CountInstructionTokensResponseObject, error) {
	if len(auth.VisibleScopes(ctx, domain.PermAgentPublish)) == 0 {
		return openapi.CountInstructionTokens403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, domain.Scope{}),
		}, nil
	}
	if s.tokenisers == nil || req.Body == nil {
		return nil, errNoAdministration
	}
	if len(req.Body.Instructions) > longestInstruction {
		return openapi.CountInstructionTokens400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("the instruction is too long to count")),
		}, nil
	}

	at, refused := countScope(ctx, req.Body.Scope)
	if refused != nil {
		return refused, nil
	}

	answer := openapi.CountInstructionTokens200JSONResponse{Model: req.Body.Model}

	counter, err := s.tokenisers.Counter(req.Body.Provider, at, model.Config{Model: req.Body.Model})
	if errors.Is(err, model.ErrNoTokeniser) {
		// A state, not a failure: this provider has no counting endpoint.
		// Answered as an answer, so the console can say "characters" and mean
		// it rather than showing a number nobody measured.
		return answer, nil
	}
	if err != nil {
		return nil, err
	}

	tokens, err := counter.Count(ctx, req.Body.Instructions)
	if err != nil {
		// A provider that is configured but not reachable is a real failure
		// and is reported as one. Folding it into "cannot count" would hide a
		// wrong credential behind a sentence about tokenisers.
		return nil, err
	}

	answer.Counted, answer.Tokens = true, &tokens
	return answer, nil
}

// countScope is the scope an instruction is counted at: the agent's, which the
// caller must be able to publish in, since the count reaches a provider with
// that scope's key; or, with none named, the installation's (NT-019).
func countScope(ctx context.Context, raw *string) (domain.Scope, openapi.CountInstructionTokensResponseObject) {
	if raw == nil || *raw == "" {
		return domain.Scope{Company: domain.Installation}, nil
	}
	target, err := parseTarget(*raw)
	if err != nil {
		return domain.Scope{}, openapi.CountInstructionTokens400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(invalid(err.Error())),
		}
	}
	if err := auth.Require(ctx, domain.PermAgentPublish, target); err != nil {
		return domain.Scope{}, openapi.CountInstructionTokens403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, target),
		}
	}
	return target, nil
}
