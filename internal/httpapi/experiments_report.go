package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
)

// errNoExperiment is an id that names no experiment of the agent on the route.
var errNoExperiment = errors.New("no experiment of this agent has that id")

// GetExperiment folds an experiment's runs against the dataset version it pinned.
func (s *Server) GetExperiment(
	ctx context.Context, req openapi.GetExperimentRequestObject,
) (openapi.GetExperimentResponseObject, error) {
	published, ok, err := s.publishedAgent(ctx, req.AgentId)
	if err != nil || !ok {
		return openapi.GetExperiment404ApplicationProblemPlusJSONResponse(
			problemOf(notFound(req.AgentId))), err
	}
	if err := auth.Require(ctx, domain.PermAgentRead, published.Scope); err != nil {
		return openapi.GetExperiment403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentRead, published.Scope),
		}, nil
	}
	report, err := s.experimentReport(ctx, published.ID, req.ExperimentId)
	if errors.Is(err, errNoExperiment) {
		return openapi.GetExperiment404ApplicationProblemPlusJSONResponse(
			problemOf(notFound(req.ExperimentId))), nil
	}
	if err != nil {
		return nil, err
	}
	return openapi.GetExperiment200JSONResponse(s.experimentReportOut(ctx, report)), nil
}

// CompareExperiments answers what changed from one experiment of an agent to another.
func (s *Server) CompareExperiments(
	ctx context.Context, req openapi.CompareExperimentsRequestObject,
) (openapi.CompareExperimentsResponseObject, error) {
	published, ok, err := s.publishedAgent(ctx, req.AgentId)
	if err != nil || !ok {
		return openapi.CompareExperiments404ApplicationProblemPlusJSONResponse(
			problemOf(notFound(req.AgentId))), err
	}
	if err := auth.Require(ctx, domain.PermAgentRead, published.Scope); err != nil {
		return openapi.CompareExperiments403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentRead, published.Scope),
		}, nil
	}
	sides := make([]simulate.ExperimentReport, 0, 2)
	for _, id := range []string{req.Params.From, req.Params.To} {
		report, err := s.experimentReport(ctx, published.ID, id)
		if errors.Is(err, errNoExperiment) {
			return openapi.CompareExperiments404ApplicationProblemPlusJSONResponse(problemOf(notFound(id))), nil
		}
		if err != nil {
			return nil, err
		}
		sides = append(sides, report)
	}
	return openapi.CompareExperiments200JSONResponse(
		experimentComparisonOut(simulate.CompareExperiments(sides[0], sides[1]))), nil
}

/*
experimentReport reads one experiment of agent: its record, its runs, and the corpus of the
dataset version it pinned.

The agent is checked against the record, not the id: an id is whatever a caller typed, and
an experiment of another agent behind this agent's route would be read under this agent's
grants.
*/
func (s *Server) experimentReport(
	ctx context.Context, agent domain.AgentID, id string,
) (simulate.ExperimentReport, error) {
	record, found, err := simulate.Recorded(ctx, s.store, id)
	if err != nil {
		return simulate.ExperimentReport{}, fmt.Errorf("experiment %s: %w", id, err)
	}
	if !found || record.Agent != agent {
		return simulate.ExperimentReport{}, errNoExperiment
	}
	runs, err := simulate.Gather(ctx, s.store, id)
	if err != nil {
		return simulate.ExperimentReport{}, fmt.Errorf("experiment %s: %w", id, err)
	}
	var corpus []domain.RegressionCase
	if s.regressions != nil {
		if corpus, err = s.regressions.AtVersion(ctx, agent, record.DatasetVersion); err != nil {
			return simulate.ExperimentReport{}, fmt.Errorf("dataset version %d of %s: %w",
				record.DatasetVersion, agent, err)
		}
	}
	return simulate.ReadExperiment(record, runs, corpus), nil
}

func problemOf(p openapi.NotFoundApplicationProblemPlusJSONResponse) openapi.Problem {
	return openapi.Problem(p)
}
