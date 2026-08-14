package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
)

/*
Two versions of one agent on the same corrections.

Nothing is stored for it. Each side is the newest battery run against that
version — a set of simulated runs naming it — and the diff is a fold of two
folds, so there is no third account of the same events to fall out of step
with the ledger.
*/

// CompareVersions answers what changed between two versions.
func (s *Server) CompareVersions(
	ctx context.Context, req openapi.CompareVersionsRequestObject,
) (openapi.CompareVersionsResponseObject, error) {
	absent := openapi.CompareVersions404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}
	published, ok, err := s.publishedAgent(ctx, req.AgentId)
	if err != nil || !ok {
		return absent, err
	}
	if err := auth.Require(ctx, domain.PermAgentRead, published.Scope); err != nil {
		return openapi.CompareVersions403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentRead, published.Scope),
		}, nil
	}

	from, to, err := s.twoVersions(ctx, published, req.Params)
	if err != nil {
		return openapi.CompareVersions409ApplicationProblemPlusJSONResponse(
			conflicted(err.Error())), nil
	}

	was, err := s.battery(ctx, published.ID, from)
	if err != nil {
		return comparisonRefused(err)
	}
	now, err := s.battery(ctx, published.ID, to)
	if err != nil {
		return comparisonRefused(err)
	}

	return openapi.CompareVersions200JSONResponse(
		comparisonOf(simulate.Compare(was, now))), nil
}

// twoVersions resolves what to compare: what was asked for, or the current
// version against the one published before it, which is the question at the
// moment somebody publishes.
func (s *Server) twoVersions(
	ctx context.Context, published domain.AgentSummary, params openapi.CompareVersionsParams,
) (from, to domain.VersionID, err error) {
	to = published.VersionID
	if params.To != nil && *params.To != "" {
		to = domain.VersionID(*params.To)
	}
	if params.From != nil && *params.From != "" {
		return domain.VersionID(*params.From), to, nil
	}

	versions, err := s.agents.Versions(ctx, published.ID)
	if err != nil {
		return "", "", fmt.Errorf("agent versions: %w", err)
	}
	for at, v := range versions {
		if v.VersionID == to && at+1 < len(versions) {
			return versions[at+1].VersionID, to, nil
		}
	}
	return "", "", fmt.Errorf(
		"%s has nothing published before %s to compare it with", published.ID, to)
}

// battery folds the newest battery run against one version, read against the
// corrections it was meant to check.
func (s *Server) battery(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) (simulate.Report, error) {
	if s.batteries == nil || s.regressions == nil {
		return simulate.Report{}, fmt.Errorf("this installation keeps no corrections")
	}
	simulation, found, err := s.batteries.Latest(ctx, agent, version)
	if err != nil {
		return simulate.Report{}, fmt.Errorf("last battery of %s: %w", version, err)
	}
	if !found {
		// Never an empty diff: that reads as "nothing changed" about two
		// versions nobody ever compared.
		return simulate.Report{}, fmt.Errorf(
			"the corpus has never been run against %s", version)
	}

	report, err := simulate.Gather(ctx, s.store, simulation)
	if err != nil {
		return simulate.Report{}, fmt.Errorf("simulation %s: %w", simulation, err)
	}
	corpus, err := s.regressions.List(ctx, agent)
	if err != nil {
		return simulate.Report{}, fmt.Errorf("read the corpus of %s: %w", agent, err)
	}
	report.Version = version
	return simulate.Battery(report, corpus), nil
}

func comparisonRefused(err error) (openapi.CompareVersionsResponseObject, error) {
	return openapi.CompareVersions409ApplicationProblemPlusJSONResponse(
		conflicted(err.Error())), nil
}

func comparisonOf(c simulate.Comparison) openapi.VersionComparison {
	out := openapi.VersionComparison{
		From: string(c.From), To: string(c.To),
		Regressed: c.Regressed, Fixed: c.Fixed, CostMicros: c.CostMicros,
		Cases: make([]openapi.CaseChange, 0, len(c.Cases)),
	}
	for _, change := range c.Cases {
		out.Cases = append(out.Cases, openapi.CaseChange{
			Id:         change.ID,
			Was:        openapi.Standing(change.Was),
			Now:        openapi.Standing(change.Now),
			CostMicros: change.CostMicros,
			Steps:      change.Steps,
		})
	}
	return out
}
