package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/flow"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
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

	catalogue, err := s.rulingsFor(ctx, specScope(draft))
	if err != nil {
		return nil, err
	}

	finding := flow.Check(draft.Tools, envelopesOf(draft), catalogue)
	return openapi.CheckDataFlow200JSONResponse(findingFrom(finding)), nil
}

// envelopesOf hands the checker the steps as data. The flow package cannot
// import the one that parses definitions — dependencies point inward.
func envelopesOf(draft spec.Spec) []flow.Envelope {
	out := make([]flow.Envelope, 0, len(draft.Steps))
	for _, step := range draft.Steps {
		out = append(out, flow.Envelope{Name: step.Name, Reaches: step.Reaches})
	}
	return out
}

func specScope(draft spec.Spec) domain.Scope {
	return domain.Scope{Company: draft.Company, Area: draft.Area}
}

// catalogue is what the Curator has ruled about each tool.
type ruled map[domain.ToolID]domain.ToolEntry

func (r ruled) Effect(tool domain.ToolID) (domain.Effect, bool) {
	entry, ok := r[tool]
	if !ok || !entry.Effect.Valid() {
		return domain.EffectUnknown, false
	}
	return entry.Effect, true
}

func (r ruled) Untrusted(tool domain.ToolID) bool { return r[tool].Untrusted }

// rulings is the catalogue with what the Curator decided about each tool. The
// assistant's own catalogue next door needs only the names, which is why there
// are two: one answers "what may I mention", this one answers "what does it do".
func (s *Server) rulingsFor(ctx context.Context, scope domain.Scope) (ruled, error) {
	if s.tools == nil {
		return ruled{}, nil
	}
	entries, err := s.tools.Tools(ctx)
	if err != nil {
		return nil, fmt.Errorf("tool catalogue: %w", err)
	}
	out := make(ruled, len(entries))
	for _, entry := range entries {
		if entry.Native && !entry.Scope.Contains(scope) {
			continue
		}
		out[entry.ID] = entry
	}
	return out, nil
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
