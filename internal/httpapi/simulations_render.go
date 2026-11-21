package httpapi

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
)

// Rendering a simulation's report: the fold of its runs, as the API answers it.

func (s *Server) simulationReport(ctx context.Context, r simulate.Report) openapi.SimulationReport {
	out := openapi.SimulationReport{
		Id: r.ID, Running: r.Running,
		Held: ptr(r.Held), Broken: ptr(r.Broken), Drifted: ptr(r.Drifted), Erased: ptr(r.Erased),
		Cases: make([]openapi.SimulationCase, 0, len(r.Cases)),
	}
	if r.Agent != "" {
		out.Agent = ptr(string(r.Agent))
	}
	if r.Version != "" {
		out.Version = ptr(string(r.Version))
	}
	for _, c := range r.Cases {
		out.Cases = append(out.Cases, s.simulationCase(ctx, c))
	}
	return out
}

/*
simulationCase renders one case, resolving the answer wherever it lives.

A case from a run recorded before the answer moved carries it inline; one
recorded since carries a reference. An answer that was erased is reported as
erased rather than as an empty string, because a report that blanks it says the
agent finished silently — which is a different result, and the one a reader
would act on.
*/
func (s *Server) simulationCase(ctx context.Context, c simulate.Case) openapi.SimulationCase {
	outcome := c.Outcome
	var outcomeState *openapi.SimulationOutcomeState
	if c.OutcomeRef != "" {
		resolved, err := engine.OutcomeOf(ctx, s.content, domain.RunFinishedPayload{
			OutcomeRef: c.OutcomeRef,
		})
		if err != nil {
			unavailable := openapi.SimulationOutcomeUnavailable
			outcome, outcomeState = "", &unavailable
		} else {
			outcome = resolved
		}
	}
	return s.caseOut(c, outcome, outcomeState)
}

func (s *Server) caseOut(
	c simulate.Case, outcome string, outcomeState *openapi.SimulationOutcomeState,
) openapi.SimulationCase {
	out := openapi.SimulationCase{
		Id:           someString(c.ID),
		Settled:      openapi.SimulationSettled(c.Settled),
		Steps:        c.Steps,
		Cost:         toCost(c.Spent()),
		RunId:        someString(string(c.RunID)),
		Outcome:      someString(outcome),
		OutcomeState: outcomeState,
		Reason:       someString(c.Reason),
		Model:        someString(c.Model),
	}
	if c.Drifted {
		out.Drifted = ptr(true)
	}
	if len(c.Expected) > 0 {
		expected := toExpectations(c.Expected)
		out.Expected = &expected
	}
	if len(c.Unmet) > 0 {
		unmet := toExpectations(c.Unmet)
		out.Unmet = &unmet
	}
	if every := c.EveryAct(); len(every) > 0 {
		acts := make([]openapi.SimulationAct, 0, len(every))
		for _, a := range every {
			acts = append(acts, toSimulationAct(a))
		}
		out.Acted = &acts
	}
	return out
}

func toSimulationAct(a simulate.Act) openapi.SimulationAct {
	return openapi.SimulationAct{
		Tool:    string(a.Tool),
		Effect:  openapi.ToolEffect(a.Effect.String()),
		Verdict: openapi.Verdict(a.Verdict.String()),
		Reached: a.Reached,
		Step:    someString(a.Step),
		Rule:    someString(a.Rule),
		Policy:  someString(a.Policy),
		Reason:  someString(a.Reason),
	}
}
