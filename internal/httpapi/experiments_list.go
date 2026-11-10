package httpapi

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
)

// listedExperiments is how many of an agent's experiments one page lists, newest first.
const listedExperiments = 20

/*
ListExperiments is an agent's newest experiments, by what each pinned.

Each one is read from its own runs and shown only where they were recorded, as its report
is: an agent republished elsewhere keeps its old experiments there, and a reader of where it
is published now is not shown them. Leaving one out can make a page shorter than the limit,
which is a page, not an error.
*/
func (s *Server) ListExperiments(
	ctx context.Context, req openapi.ListExperimentsRequestObject,
) (openapi.ListExperimentsResponseObject, error) {
	published, ok, err := s.publishedAgent(ctx, req.AgentId)
	if err != nil || !ok {
		return openapi.ListExperiments404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
		}, err
	}
	if err := auth.Require(ctx, domain.PermAgentRead, published.Scope); err != nil {
		return openapi.ListExperiments403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentRead, published.Scope),
		}, nil
	}
	ids, err := s.store.Experiments(ctx, published.ID, listedExperiments)
	if err != nil {
		return nil, fmt.Errorf("experiments of %s: %w", published.ID, err)
	}
	out := openapi.ExperimentList{Experiments: make([]openapi.ExperimentListing, 0, len(ids))}
	for _, id := range ids {
		record, found, err := simulate.Recorded(ctx, s.store, id)
		if err != nil {
			return nil, fmt.Errorf("experiment %s: %w", id, err)
		}
		if !found || record.Agent != published.ID || auth.Require(ctx, domain.PermAgentRead, record.Scope) != nil {
			continue
		}
		out.Experiments = append(out.Experiments, experimentListingOut(id, record))
	}
	return openapi.ListExperiments200JSONResponse(out), nil
}

func experimentListingOut(id string, r simulate.Record) openapi.ExperimentListing {
	return openapi.ExperimentListing{
		Id: id, OpenedAt: r.OpenedAt, DatasetVersion: r.DatasetVersion, Version: string(r.Version),
		Config: experimentConfigOut(r.Config), Repetitions: r.Repetitions,
		Runs: len(r.Runs), Cases: r.Cases,
	}
}
