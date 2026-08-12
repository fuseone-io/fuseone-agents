package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
)

// Simulations is the queue the API hands work to, declared here by the
// consumer. The API never drives a simulation itself: a set is minutes of
// model calls, and a request is the wrong thing to hold open for it.
type Simulations interface {
	Submit(job simulate.Job) error
	Report(ctx context.Context, id string) (simulate.Report, error)
}

// Resolve turns an agent version into something that can be simulated.
//
// A function rather than an interface: there is one method, and the only
// implementation is four lines of adapter in the wiring. Declaring an
// interface for it would make this package name the one that resolves
// definitions, and the dependencies point the other way.
type Resolve func(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) (engine.Start, engine.Planner, error)

// Cases is where an uploaded set is filed: the ledger's claim check, under the
// same retention as every other bulky payload (AU-04).
type Cases interface {
	PutFor(ctx context.Context, kind, owner string, seq int64, data []byte) (string, error)
}

// WithSimulations wires the authoring safety net. Optional, like the rest of
// the authoring area: an installation that publishes agents from files has no
// use for it.
func (s *Server) WithSimulations(sims Simulations, resolve Resolve, cases Cases) *Server {
	s.simulations, s.resolve, s.cases = sims, resolve, cases
	return s
}

// StartSimulation accepts a set of occurrences and answers before running it.
func (s *Server) StartSimulation(
	ctx context.Context, req openapi.StartSimulationRequestObject,
) (openapi.StartSimulationResponseObject, error) {
	absent := openapi.StartSimulation404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}
	published, ok, err := s.publishedAgent(ctx, req.AgentId)
	if err != nil || !ok {
		return absent, err
	}

	// Simulating spends real money at a real provider, and it is the gate an
	// agent passes before it may be published (FU-10). Reading runs is not
	// that authority.
	if err := auth.Require(ctx, domain.PermAgentPublish, published.Scope); err != nil {
		return openapi.StartSimulation403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentPublish, published.Scope),
		}, nil
	}
	if s.simulations == nil || s.resolve == nil || s.cases == nil {
		return absent, nil
	}

	cases, err := simulate.Load(ctx, s.cases, published.ID, []byte(req.Body.Cases))
	if err != nil {
		// The whole file, named line and all. Running forty-nine of fifty and
		// mentioning nothing gives an author coverage that is a lie.
		return openapi.StartSimulation400ApplicationProblemPlusJSONResponse(
			problem(http.StatusBadRequest, "Case set refused", err.Error())), nil
	}

	start, planner, err := s.resolve(ctx, published.ID, published.VersionID)
	if err != nil {
		return nil, fmt.Errorf("resolve %s@%s: %w", published.ID, published.VersionID, err)
	}

	job := simulate.Job{
		ID:      simulationID(published.ID, clockOr(s.clock).Now().UnixMilli(), req.Params.IdempotencyKey),
		Agent:   published.ID,
		Version: published.VersionID,
		Start:   start,
		Planner: planner,
		Cases:   cases,
	}
	if err := s.simulations.Submit(job); err != nil {
		if errors.Is(err, simulate.ErrBusy) {
			return openapi.StartSimulation409ApplicationProblemPlusJSONResponse(problem(
				http.StatusConflict, "Another simulation is running",
				"These cost real money at a real provider, so they are not queued without limit.",
			)), nil
		}
		return nil, fmt.Errorf("submit simulation: %w", err)
	}

	return openapi.StartSimulation202JSONResponse{Id: job.ID, Cases: len(cases)}, nil
}

// GetSimulation folds the runs the simulation opened back into rows.
func (s *Server) GetSimulation(
	ctx context.Context, req openapi.GetSimulationRequestObject,
) (openapi.GetSimulationResponseObject, error) {
	absent := openapi.GetSimulation404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}
	published, ok, err := s.publishedAgent(ctx, req.AgentId)
	if err != nil || !ok {
		return absent, err
	}
	if err := auth.Require(ctx, domain.PermAgentRead, published.Scope); err != nil {
		return openapi.GetSimulation403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermAgentRead, published.Scope),
		}, nil
	}
	if s.simulations == nil {
		return absent, nil
	}

	report, err := s.simulations.Report(ctx, req.SimulationId)
	if err != nil {
		return nil, fmt.Errorf("simulation %s: %w", req.SimulationId, err)
	}
	return openapi.GetSimulation200JSONResponse(toSimulationReport(report)), nil
}

// publishedAgent resolves the newest published version this caller may see.
func (s *Server) publishedAgent(ctx context.Context, id string) (domain.AgentSummary, bool, error) {
	if s.agents == nil {
		return domain.AgentSummary{}, false, nil
	}
	versions, err := s.agents.Versions(ctx, domain.AgentID(id))
	if err != nil {
		return domain.AgentSummary{}, false, fmt.Errorf("agent versions: %w", err)
	}
	if len(versions) == 0 || !readable(versions[0].Scope, auth.VisibleScopes(ctx, domain.PermAgentRead)) {
		return domain.AgentSummary{}, false, nil
	}
	return versions[0], true, nil
}

// simulationID names the batch after the intention that started it, the same
// way a run is named: a caller that never saw the answer asks again and
// reaches the simulation it already started rather than paying for a second.
func simulationID(agent domain.AgentID, at int64, key string) string {
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("sim_%s_%d_%s", agent, at, hex.EncodeToString(sum[:4]))
}

// --- rendering -------------------------------------------------------------

func toSimulationReport(r simulate.Report) openapi.SimulationReport {
	out := openapi.SimulationReport{
		Id: r.ID, Expected: r.Expected, Running: r.Running,
		Cases: make([]openapi.SimulationCase, 0, len(r.Cases)),
	}
	if r.Agent != "" {
		out.Agent = ptr(string(r.Agent))
	}
	if r.Version != "" {
		out.Version = ptr(string(r.Version))
	}
	for _, c := range r.Cases {
		out.Cases = append(out.Cases, toSimulationCase(c))
	}
	return out
}

func toSimulationCase(c simulate.Case) openapi.SimulationCase {
	out := openapi.SimulationCase{
		Settled: openapi.SimulationSettled(c.Settled),
		Steps:   c.Steps,
		Cost:    toCost(c.Cost),
	}
	out.RunId = someString(string(c.RunID))
	out.Outcome = someString(c.Outcome)
	out.Reason = someString(c.Reason)
	out.Error = someString(c.Error)

	if len(c.Acted) > 0 {
		acts := make([]openapi.SimulationAct, 0, len(c.Acted))
		for _, a := range c.Acted {
			acts = append(acts, toSimulationAct(a))
		}
		out.Acted = &acts
	}
	return out
}

func toSimulationAct(a simulate.Act) openapi.SimulationAct {
	out := openapi.SimulationAct{
		Tool:    string(a.Tool),
		Effect:  openapi.Effect(a.Effect.String()),
		Verdict: openapi.Verdict(a.Verdict.String()),
		Reached: a.Reached,
	}
	out.Step = someString(a.Step)
	out.Rule = someString(a.Rule)
	out.Reason = someString(a.Reason)
	return out
}

// someString renders an optional field, and renders nothing rather than an
// empty string: a rule of "" beside a verdict reads as a rule nobody named.
func someString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
