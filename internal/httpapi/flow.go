package httpapi

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/flow"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/publishable"
)

/*
CheckDataFlow answers, from the definition alone, where data this installation
did not author reaches a tool that acts on the world (PRD SE-07).

It reports and does not refuse. The path it finds is usually the point of the
agent, and a check that blocked publication would be switched off within a
week. What it buys is that nobody is surprised on Monday by an approval queue
they did not expect.

Authorised as publishing is: it answers a question about a draft the caller is
about to publish, and an author who may not publish into an area has no reason
to be asking what publishing into it would do.
*/
func (s *Server) CheckDataFlow(ctx context.Context, req openapi.CheckDataFlowRequestObject) (openapi.CheckDataFlowResponseObject, error) {
	scope, allowed := publishScope(ctx,
		domain.CompanyID(req.Body.Company), domain.AreaID(req.Body.Area))
	if !allowed {
		return openapi.CheckDataFlow403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(
				domain.PermAgentPublish, scope),
		}, nil
	}

	_, draft, err := renderAndParse(req.AgentId, *req.Body)
	if err != nil {
		return openapi.CheckDataFlow400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}

	catalogue, err := s.checker().Rulings(ctx, publishable.ScopeOf(draft))
	if err != nil {
		return nil, err
	}

	finding := flow.Check(draft.Tools, publishable.Envelopes(draft), catalogue)
	return openapi.CheckDataFlow200JSONResponse(findingFrom(finding)), nil
}

// checker is the publication's judgement of a definition against this
// installation, shared by every way of publishing (NT-018, contract 6).
func (s *Server) checker() publishable.Checker {
	return publishable.Checker{Providers: s.providerNames, Tools: s.tools}
}

func findingFrom(f flow.Finding) openapi.FlowFinding {
	out := openapi.FlowFinding{
		Paths:        make([]openapi.FlowPath, 0, len(f.Paths)),
		Unclassified: make([]string, 0, len(f.Unclassified)),
	}
	for _, p := range f.Paths {
		path := openapi.FlowPath{
			From: string(p.From), To: string(p.To),
			Effect: openapi.Effect(p.Effect.String()),
		}
		if p.FromStep != "" {
			path.FromStep = ptr(p.FromStep)
		}
		if p.ToStep != "" {
			path.ToStep = ptr(p.ToStep)
		}
		out.Paths = append(out.Paths, path)
	}
	for _, tool := range f.Unclassified {
		out.Unclassified = append(out.Unclassified, string(tool))
	}
	return out
}
