package httpapi

import (
	gocontext "context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/simulate"
)

/*
A replayed case that left its world reads as such at the door (NT-018 phase 5): the row names
the divergence, a battery counts it apart from held and broken, and so does an experiment and
each of its cases. A row that said nothing would read as a case that held.
*/

func TestSimulationCase_aCaseThatLeftItsWorld_namesTheDivergence(t *testing.T) {
	t.Parallel()
	got := NewServer(ledger.NewMemory(), "test").simulationCase(gocontext.Background(), simulate.Case{
		ID: "estorno", RunID: "run-1", Settled: simulate.SettledFinished,
		Diverged: &domain.ReplayDivergedPayload{Position: 2, Divergence: domain.DivergenceOutOfOrder, Tool: "kb.search"},
	})
	want := openapi.ReplayDivergence{Position: 2, Divergence: openapi.ReplayDivergenceDivergenceOutOfOrder, Tool: "kb.search"}
	if got.Diverged == nil || *got.Diverged != want {
		t.Errorf("diverged = %+v, want %+v", got.Diverged, want)
	}
}

func TestReports_countTheCasesThatLeftTheirWorld(t *testing.T) {
	t.Parallel()
	report := NewServer(ledger.NewMemory(), "test").simulationReport(gocontext.Background(),
		simulate.Report{ID: "sim-1", NotComparable: 3})
	if report.NotComparable == nil || *report.NotComparable != 3 {
		t.Errorf("simulation notComparable = %v, want 3", report.NotComparable)
	}
	summary := experimentSummaryOut(simulate.ExperimentSummary{NotComparable: 2})
	if summary.NotComparable != 2 {
		t.Errorf("experiment notComparable = %d, want 2", summary.NotComparable)
	}
}
