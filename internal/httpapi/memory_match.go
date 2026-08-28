package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	memstore "github.com/fuseone/agents/internal/memory"
)

/*
What the platform already holds for an identity, asked before it is taught
again.

Read permission rather than publish. It returns rows a list already returns, so
asking for more would refuse the question to somebody who can read the answer
another way — and the point is to be asked, on every keystroke, by whoever is
looking at the form.

Nothing here writes, including the keyless rows it finds: repairing one is the
sweep's job and the next write's, and this is a read.
*/
func (s *Server) MatchMemory(
	ctx context.Context, req openapi.MatchMemoryRequestObject,
) (openapi.MatchMemoryResponseObject, error) {
	if s.memory == nil || req.Body == nil {
		return badMemoryMatch("memory match body is required"), nil
	}
	scope, err := inputScope(req.Body.Company, req.Body.Area)
	if err != nil {
		return badMemoryMatch(err.Error()), nil
	}
	if err := auth.Require(ctx, domain.PermAgentRead, scope); err != nil {
		return openapi.MatchMemory403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentRead, scope),
		}, nil
	}
	agent, refused := matchNamespace(*req.Body)
	if refused != nil {
		return badMemoryMatch(*refused), nil
	}

	found, err := s.memory.Match(ctx, memstore.MatchInput{
		Scope: scope, AgentID: agent, Kind: req.Body.Kind,
		Subject: req.Body.Subject, Signature: req.Body.Signature,
		Now: clockOr(s.clock).Now(),
	})
	switch memoryRefusal(err) {
	case http.StatusConflict:
		return openapi.MatchMemory409ApplicationProblemPlusJSONResponse(
			conflicted(err.Error())), nil
	case http.StatusBadRequest:
		return badMemoryMatch(err.Error()), nil
	}
	if err != nil {
		return nil, fmt.Errorf("match memory: %w", err)
	}
	return openapi.MatchMemory200JSONResponse(memoryMatch(found)), nil
}

/*
matchNamespace reads which namespace is being asked about.

Asked for rather than derived, unlike creation: there is no evidence yet to read
an agent from, because nothing has been composed. So the two have to agree
between themselves — a shared question naming an agent means somebody built the
request wrong, and answering it as either one would answer a question nobody
asked.
*/
func matchNamespace(in openapi.MemoryMatchInput) (domain.AgentID, *string) {
	agent := domain.AgentID(valueOr(in.AgentId))
	switch {
	case !in.Namespace.Valid():
		return "", ptr("namespace must be agent or shared")
	case in.Namespace == openapi.MemoryMatchInputNamespaceShared && agent != "":
		return "", ptr("shared memory belongs to no agent; leave agentId out")
	case in.Namespace == openapi.MemoryMatchInputNamespaceAgent && agent == "":
		return "", ptr("agentId is required to ask about an agent's own memory")
	}
	return agent, nil
}

func memoryMatch(in memstore.Match) openapi.MemoryMatch {
	out := openapi.MemoryMatch{}
	if in.Own != nil {
		own := memoryAssertion(*in.Own)
		out.Own = &own
	}
	if in.Shared != nil {
		shared := memoryAssertion(*in.Shared)
		out.Shared = &shared
	}
	if in.Pending != nil {
		pending := memorySuggestion(*in.Pending)
		out.Pending = &pending
	}
	return out
}

func badMemoryMatch(detail string) openapi.MatchMemory400ApplicationProblemPlusJSONResponse {
	return openapi.MatchMemory400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			invalid(detail)),
	}
}
