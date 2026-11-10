package httpapi

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/simulate"
)

func (s *Server) experimentReportOut(ctx context.Context, r simulate.ExperimentReport) openapi.ExperimentReport {
	out := openapi.ExperimentReport{
		Summary: experimentSummaryOut(r.ExperimentSummary),
		Cases:   make([]openapi.ExperimentCase, 0, len(r.Cases)),
	}
	for _, c := range r.Cases {
		out.Cases = append(out.Cases, openapi.ExperimentCase{
			Case: s.simulationCase(ctx, c.Case), BrokenIn: c.BrokenIn, Ran: c.Ran,
		})
	}
	return out
}

func experimentSummaryOut(r simulate.ExperimentSummary) openapi.ExperimentSummary {
	return openapi.ExperimentSummary{
		Id: r.ID, DatasetVersion: r.DatasetVersion, Version: string(r.Version),
		Config:   experimentConfigOut(r.Config),
		CasesRun: r.CasesRun, CasesInVersion: r.CasesInVersion, Erased: r.Erased,
		Repetitions: r.Repetitions, Held: r.Held, Broken: r.Broken,
		Cost: toCost(r.Cost), Running: r.Running,
	}
}

// experimentConfigOut renders only what the experiment overrode: an absent field is the
// version's own, and an empty string beside it would read as an override to nothing.
func experimentConfigOut(c domain.ExperimentConfig) openapi.ExperimentConfig {
	return openapi.ExperimentConfig{
		Provider: someString(c.Provider), Model: someString(c.Model), Effort: someString(c.Effort),
	}
}

func experimentComparisonOut(c simulate.ExperimentComparison) openapi.ExperimentComparison {
	out := openapi.ExperimentComparison{
		From: experimentSummaryOut(c.From), To: experimentSummaryOut(c.To),
		SharedOnly: c.SharedOnly, OnlyFrom: c.OnlyFrom, OnlyTo: c.OnlyTo,
		Regressed: c.Regressed, Fixed: c.Fixed, CostMicros: c.CostMicros,
		Cases: make([]openapi.ExperimentCaseChange, 0, len(c.Cases)),
	}
	for _, change := range c.Cases {
		out.Cases = append(out.Cases, openapi.ExperimentCaseChange{
			Id: change.ID, Was: openapi.Standing(change.Was), Now: openapi.Standing(change.Now),
			CostMicros: change.CostMicros, Steps: change.Steps,
			WasBrokenIn: change.WasBrokenIn, WasRan: change.WasRan,
			NowBrokenIn: change.NowBrokenIn, NowRan: change.NowRan,
		})
	}
	return out
}
