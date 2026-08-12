package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
)

// Cases is where an uploaded set is filed: the ledger's claim check, under the
// same retention as every other bulky payload (AU-04).
type Cases interface {
	PutFor(ctx context.Context, kind, owner string, seq int64, data []byte) (string, error)
}

// WithCases wires where simulation sets are filed. Optional, like the rest of
// the authoring area: an installation that publishes agents from files has no
// use for it.
func (s *Server) WithCases(cases Cases) *Server {
	s.cases = cases
	return s
}

// StartSimulation opens one run per case and answers.
//
// Nothing is driven here. The runs are the queue: a simulated run is claimed
// by the pool built with the dry tool layer and advanced turn by turn like
// every other run. Opening is an append per case with no model call, which is
// why it fits inside the request and why the number answered is the number
// that actually opened.
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
	if s.cases == nil {
		return absent, nil
	}

	id := simulationID(published.ID, clockOr(s.clock).Now().UnixMilli(), req.Params.IdempotencyKey)
	cases, err := simulate.Load(ctx, s.cases, id, []byte(req.Body.Cases))
	if err != nil {
		// The whole file, named line and all. Running forty-nine of fifty and
		// mentioning nothing gives an author coverage that is a lie.
		return openapi.StartSimulation400ApplicationProblemPlusJSONResponse(
			problem(http.StatusBadRequest, "Case set refused", err.Error())), nil
	}

	opened, err := simulate.Open(ctx, s.opener(), simulate.Batch{
		ID: id, Agent: published.ID, By: callerOf(ctx), Cases: cases,
	})
	if err != nil {
		return nil, fmt.Errorf("open simulation %s: %w", id, err)
	}
	for _, why := range opened.Failed {
		slog.Warn("simulated case did not open", "simulation", id, "agent", published.ID, "err", why)
	}
	if len(opened.Runs) == 0 {
		// Every case refused for the same reason — a paused agent, most
		// likely. Answering 202 with nothing behind it would leave the author
		// polling a report that will never have a row.
		return openapi.StartSimulation409ApplicationProblemPlusJSONResponse(problem(
			http.StatusConflict, "Nothing could be simulated", firstOr(opened.Failed, "no case opened a run"),
		)), nil
	}

	return openapi.StartSimulation202JSONResponse{Id: id, Cases: len(opened.Runs)}, nil
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

	report, err := simulate.Gather(ctx, s.store, req.SimulationId)
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

func firstOr(reasons []string, fallback string) string {
	if len(reasons) == 0 {
		return fallback
	}
	return reasons[0]
}

// --- rendering -------------------------------------------------------------

func toSimulationReport(r simulate.Report) openapi.SimulationReport {
	out := openapi.SimulationReport{
		Id: r.ID, Running: r.Running,
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
		RunId:   someString(string(c.RunID)),
		Outcome: someString(c.Outcome),
		Reason:  someString(c.Reason),
	}
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
	return openapi.SimulationAct{
		Tool:    string(a.Tool),
		Effect:  openapi.Effect(a.Effect.String()),
		Verdict: openapi.Verdict(a.Verdict.String()),
		Reached: a.Reached,
		Step:    someString(a.Step),
		Rule:    someString(a.Rule),
		Reason:  someString(a.Reason),
	}
}

// someString renders an optional field, and renders nothing rather than an
// empty string: a rule of "" beside a verdict reads as a rule nobody named.
func someString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
