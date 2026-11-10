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

Read with the experiments, in one pass over their runs, and never run by run. Each one is
shown only where its runs were recorded, as its report is: an agent republished elsewhere keeps its old experiments there, and a reader of where it
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
	listed, err := s.store.Experiments(ctx, published.ID, listedExperiments)
	if err != nil {
		return nil, fmt.Errorf("experiments of %s: %w", published.ID, err)
	}
	out := openapi.ExperimentList{Experiments: make([]openapi.ExperimentListing, 0, len(listed))}
	for _, e := range listed {
		if e.Opening.AgentID != published.ID ||
			auth.Require(ctx, domain.PermAgentRead, e.Opening.Scope) != nil {
			continue
		}
		intent, err := simulate.IntentionOf(e.Opening)
		if err != nil {
			return nil, fmt.Errorf("experiment %s: %w", e.ID, err)
		}
		out.Experiments = append(out.Experiments, experimentListingOut(e, intent))
	}
	return openapi.ListExperiments200JSONResponse(out), nil
}

func experimentListingOut(e domain.ExperimentListing, intent simulate.Intention) openapi.ExperimentListing {
	return openapi.ExperimentListing{
		Id: e.ID, OpenedAt: e.OpenedAt, DatasetVersion: intent.DatasetVersion,
		Version: string(intent.Version), Config: experimentConfigOut(intent.Config),
		Repetitions: intent.Repetitions, Runs: e.Runs, Cases: e.Cases,
	}
}
