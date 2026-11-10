package simulate_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

/*
Reading an experiment (NT-015 § Experiments).

A result says what it ran before what it found: dataset version, agent version,
configuration, cases run out of the cases in the version, erased cases, repetitions, and
only then held, broken and cost. Repetitions are for structural certainty, not an
average: a case broken in any repetition is broken, and the report says in how many.
*/

func lookup() []simulate.Act {
	return []simulate.Act{{Tool: "crm.lookup", Verdict: domain.VerdictAllow, Reached: true}}
}

func expectsLookup(id string) domain.RegressionCase {
	return corpusCase(id, domain.Expectation{Kind: domain.ExpectCalls, Value: "crm.lookup"})
}

// repeated is a case run n times, held in every run except the ones named.
func repeated(id string, n int, broken ...int) []simulate.Case {
	out := make([]simulate.Case, 0, n)
	for i := range n {
		c := simulate.Case{
			ID: id, RunID: domain.RunID(id + "-" + string(rune('a'+i))), Settled: simulate.SettledFinished,
			Acted: lookup(), Cost: domain.Cost{Micros: 10},
		}
		for _, b := range broken {
			if b == i {
				c.Acted = nil
			}
		}
		out = append(out, c)
	}
	return out
}

func experimentRecord(repetitions int) simulate.Record {
	return simulate.Record{Intention: simulate.Intention{
		Experiment: simulate.Experiment{
			Version: "v2", Config: domain.ExperimentConfig{Model: "other-model"}, Repetitions: repetitions,
		},
		DatasetVersion: 3,
	}}
}

func TestReadExperiment_saysWhatItRanBeforeWhatItFound(t *testing.T) {
	t.Parallel()
	erased := corpusCase("apagado")
	erased.Erased = true
	runs := append(repeated("estorno", 5, 2), repeated("consulta", 5)...)

	got := simulate.ReadExperiment(experimentRecord(5),
		simulate.Report{ID: "exp-1", Cases: runs},
		[]domain.RegressionCase{expectsLookup("estorno"), expectsLookup("consulta"), erased})

	want := simulate.ExperimentSummary{
		ID: "exp-1", DatasetVersion: 3, Version: "v2", Config: domain.ExperimentConfig{Model: "other-model"},
		CasesRun: 2, CasesInVersion: 3, Erased: 1, Repetitions: 5,
		Held: 1, Broken: 1, Cost: domain.Cost{Micros: 100},
	}
	if got.ExperimentSummary != want {
		t.Errorf("summary = %+v\nwant      %+v", got.ExperimentSummary, want)
	}
}

func TestReadExperiment_aCaseBrokenInOneRepetition_isBrokenAndSaysInHowMany(t *testing.T) {
	t.Parallel()
	got := simulate.ReadExperiment(experimentRecord(5),
		simulate.Report{Cases: append(repeated("estorno", 5, 3), repeated("consulta", 5)...)},
		[]domain.RegressionCase{expectsLookup("estorno"), expectsLookup("consulta")})

	byID := map[string]simulate.ExperimentCase{}
	for _, c := range got.Cases {
		byID[c.ID] = c
	}
	if c := byID["estorno"]; c.BrokenIn != 1 || c.Ran != 5 || len(c.Unmet) == 0 {
		t.Errorf("estorno = broken in %d of %d, unmet %v; want 1 of 5, unmet", c.BrokenIn, c.Ran, c.Unmet)
	}
	if c := byID["consulta"]; c.BrokenIn != 0 || c.Ran != 5 {
		t.Errorf("consulta = broken in %d of %d, want 0 of 5", c.BrokenIn, c.Ran)
	}
}

// A repetition that never opened is not one that held: the row says how many ran, so
// "0 of 3" is not read as "0 of 5".
func TestReadExperiment_fewerRepetitionsThanAsked_saysHowManyRan(t *testing.T) {
	t.Parallel()
	got := simulate.ReadExperiment(experimentRecord(5),
		simulate.Report{Cases: repeated("estorno", 3)},
		[]domain.RegressionCase{expectsLookup("estorno"), expectsLookup("consulta")})

	byID := map[string]simulate.ExperimentCase{}
	for _, c := range got.Cases {
		byID[c.ID] = c
	}
	if c := byID["estorno"]; c.Ran != 3 || c.BrokenIn != 0 {
		t.Errorf("estorno = broken in %d of %d ran, want 0 of 3", c.BrokenIn, c.Ran)
	}
	if c := byID["consulta"]; c.Ran != 0 || got.Broken != 1 || got.CasesRun != 1 {
		t.Errorf("consulta ran %d, broken=%d, casesRun=%d; want a case that never ran, counted broken",
			c.Ran, got.Broken, got.CasesRun)
	}
}
