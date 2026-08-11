// Package httpapi implements the OpenAPI contract in api/openapi.yaml.
//
// The interface it satisfies is generated from that file, so an endpoint that
// drifts from the contract fails to compile rather than failing in a customer
// integration.
package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

const defaultLimit = 50

// Store is the read side this package needs: the ledger plus enough listing to
// build projections. Declared here, by the consumer.
type Store interface {
	Read(ctx context.Context, runID domain.RunID, fromSeq int64) ([]domain.Step, error)
	Append(ctx context.Context, s domain.Step) (domain.Step, error)
	Runs(ctx context.Context) ([]domain.RunID, error)
	// Stats, ListRuns and CostRollup answer questions about many runs at once.
	// They are separate from Runs and Read because answering them by folding
	// every run in the ledger does not survive an installation's second year —
	// and an append-only record is guaranteed to have one.
	Stats(ctx context.Context, filter domain.RunFilter) (domain.RunStats, error)
	ListRuns(ctx context.Context, filter domain.RunFilter, phase string, limit int) ([]domain.RunSummary, error)
	CostRollup(ctx context.Context, filter domain.RunFilter, groupBy string) ([]domain.CostBucket, error)
}

// Server implements openapi.StrictServerInterface.
type Server struct {
	store   Store
	version string

	// curator and tools back the administration area. Both are optional: an
	// installation running on the in-memory ledger has no administration
	// store, and the endpoints answer empty rather than failing.
	curator      Curator
	tools        Tools
	integrations Integrations
	agents       Agents
}

func NewServer(store Store, version string) *Server {
	return &Server{store: store, version: version}
}

// WithAdministration returns the server with the administration area wired.
// Separate from the constructor because the API serves runs whether or not an
// operator can configure anything from it.
func (s *Server) WithAdministration(curator Curator, tools Tools, integrations Integrations) *Server {
	s.curator, s.tools, s.integrations = curator, tools, integrations
	return s
}

// WithAgents wires the registry of published versions.
func (s *Server) WithAgents(agents Agents) *Server {
	s.agents = agents
	return s
}

var _ openapi.StrictServerInterface = (*Server)(nil)

func (s *Server) Health(context.Context, openapi.HealthRequestObject) (openapi.HealthResponseObject, error) {
	return openapi.Health200JSONResponse{Status: openapi.Ok, Version: s.version}, nil
}

// ListRuns reads a page from the projection.
//
// It used to read every run and fold it until enough matched the filter, which
// made a single page view cost the whole ledger. The projection exists for
// exactly this.
func (s *Server) ListRuns(ctx context.Context, req openapi.ListRunsRequestObject) (openapi.ListRunsResponseObject, error) {
	var phase string
	if req.Params.Phase != nil {
		phase = string(*req.Params.Phase)
	}

	summaries, err := s.store.ListRuns(ctx, runFilter(req.Params.Company, req.Params.Area,
		req.Params.AgentId, req.Params.Since, nil), phase, limitOf(req.Params.Limit))
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	items := make([]openapi.Run, 0, len(summaries))
	for _, summary := range summaries {
		items = append(items, runFromSummary(summary))
	}
	return openapi.ListRuns200JSONResponse{Items: items}, nil
}

func (s *Server) GetRun(ctx context.Context, req openapi.GetRunRequestObject) (openapi.GetRunResponseObject, error) {
	run, _, err := s.project(ctx, domain.RunID(req.RunId))
	if err != nil {
		if isNotFound(err) {
			return openapi.GetRun404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
			}, nil
		}
		return nil, err
	}
	return openapi.GetRun200JSONResponse(run), nil
}

func (s *Server) ListRunSteps(ctx context.Context, req openapi.ListRunStepsRequestObject) (openapi.ListRunStepsResponseObject, error) {
	from := int64(domain.FirstSeq)
	if req.Params.FromSeq != nil {
		from = *req.Params.FromSeq
	}

	steps, err := s.store.Read(ctx, domain.RunID(req.RunId), from)
	if err != nil {
		if isNotFound(err) {
			return openapi.ListRunSteps404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
			}, nil
		}
		return nil, fmt.Errorf("read steps: %w", err)
	}

	limit := limitOf(req.Params.Limit)
	page := openapi.StepPage{Items: make([]openapi.Step, 0, min(limit, len(steps)))}
	for _, st := range steps {
		if len(page.Items) == limit {
			page.NextSeq = ptr(st.Seq)
			break
		}
		page.Items = append(page.Items, toStep(st))
	}
	return openapi.ListRunSteps200JSONResponse(page), nil
}

func (s *Server) VerifyRun(ctx context.Context, req openapi.VerifyRunRequestObject) (openapi.VerifyRunResponseObject, error) {
	steps, err := s.store.Read(ctx, domain.RunID(req.RunId), domain.FirstSeq)
	if err != nil {
		if isNotFound(err) {
			return openapi.VerifyRun404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: notFound(req.RunId),
			}, nil
		}
		return nil, fmt.Errorf("read steps: %w", err)
	}

	result := openapi.VerifyResult{Valid: true, StepsChecked: int64(len(steps))}
	if err := domain.VerifyChain(steps); err != nil {
		result.Valid = false
		result.StepsChecked = int64(len(steps))
		result.BrokenAtSeq = ptr(firstBrokenSeq(steps))
	}
	return openapi.VerifyRun200JSONResponse(result), nil
}

// project folds a run and shapes it for the wire.
//
// Folding on every read is honest for a development ledger and will not
// survive real volume; Postgres will serve this from a materialised runs
// projection maintained on append.
func (s *Server) project(ctx context.Context, id domain.RunID) (openapi.Run, engine.State, error) {
	steps, err := s.store.Read(ctx, id, domain.FirstSeq)
	if err != nil {
		return openapi.Run{}, engine.State{}, err
	}
	state, err := engine.Fold(steps)
	if err != nil {
		return openapi.Run{}, engine.State{}, fmt.Errorf("fold %s: %w", id, err)
	}
	return toRun(id, state, steps), state, nil
}

func firstBrokenSeq(steps []domain.Step) int64 {
	var prev *domain.Step
	for i := range steps {
		if err := steps[i].VerifyLink(prev); err != nil {
			return steps[i].Seq
		}
		prev = &steps[i]
	}
	return 0
}

func limitOf(v *int) int {
	if v == nil || *v <= 0 {
		return defaultLimit
	}
	return *v
}

func isNotFound(err error) bool {
	return err != nil && errors.Is(err, errNotFound) ||
		(err != nil && err.Error() == "ledger: run not found")
}

var errNotFound = errors.New("not found")
