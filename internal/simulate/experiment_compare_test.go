package simulate_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

/*
Comparing experiments (NT-015 § Experiments).

Two experiments on the same dataset version asked the same questions, and compare in
full. On different versions they did not: the comparison lists only the cases both had,
and says so, with how many each side had that the other did not.
*/

func experimentOn(datasetVersion int64, repetitions int, runs []simulate.Case, corpus ...domain.RegressionCase) simulate.ExperimentReport {
	record := experimentRecord(repetitions)
	record.DatasetVersion = datasetVersion
	return simulate.ReadExperiment(record, simulate.Report{Cases: runs}, corpus)
}

func changeOf(t *testing.T, c simulate.ExperimentComparison, id string) simulate.ExperimentCaseChange {
	t.Helper()
	for _, change := range c.Cases {
		if change.ID == id {
			return change
		}
	}
	t.Fatalf("no row for %s in %+v", id, c.Cases)
	return simulate.ExperimentCaseChange{}
}

func TestCompareExperiments_onOneDatasetVersion_comparesEveryCaseWithItsRepetitions(t *testing.T) {
	t.Parallel()
	corpus := []domain.RegressionCase{expectsLookup("estorno"), expectsLookup("consulta")}
	was := experimentOn(3, 5, append(repeated("estorno", 5), repeated("consulta", 5)...), corpus...)
	now := experimentOn(3, 5, append(repeated("estorno", 5, 1, 4), repeated("consulta", 5)...), corpus...)

	got := simulate.CompareExperiments(was, now)

	if got.SharedOnly || got.Regressed != 1 || len(got.Cases) != 2 {
		t.Fatalf("comparison = %+v, want both cases, one regression, compared in full", got)
	}
	estorno := changeOf(t, got, "estorno")
	if !estorno.Regressed() || estorno.WasBrokenIn != 0 || estorno.WasRan != 5 ||
		estorno.NowBrokenIn != 2 || estorno.NowRan != 5 {
		t.Errorf("estorno = %+v, want held 0 of 5, now broken 2 of 5", estorno)
	}
}

func TestCompareExperiments_acrossDatasetVersions_listsOnlySharedCasesAndSaysSo(t *testing.T) {
	t.Parallel()
	was := experimentOn(2, 1, append(repeated("a", 1), repeated("b", 1)...), expectsLookup("a"), expectsLookup("b"))
	now := experimentOn(3, 1, append(repeated("b", 1, 0), repeated("c", 1)...), expectsLookup("b"), expectsLookup("c"))

	got := simulate.CompareExperiments(was, now)

	if !got.SharedOnly || got.OnlyFrom != 1 || got.OnlyTo != 1 {
		t.Errorf("comparison = shared only %v, left out %d and %d; want it said, one each side",
			got.SharedOnly, got.OnlyFrom, got.OnlyTo)
	}
	if len(got.Cases) != 1 || got.Cases[0].ID != "b" || !got.Cases[0].Regressed() {
		t.Errorf("cases = %+v, want only b, which regressed", got.Cases)
	}
}

// A case erased on one side was not run there, so across versions it is not shared.
func TestCompareExperiments_acrossDatasetVersions_aCaseErasedOnOneSide_isNotShared(t *testing.T) {
	t.Parallel()
	erased := corpusCase("b")
	erased.Erased = true
	was := experimentOn(2, 1, append(repeated("a", 1), repeated("b", 1)...), expectsLookup("a"), expectsLookup("b"))
	now := experimentOn(3, 1, repeated("a", 1), expectsLookup("a"), erased)

	got := simulate.CompareExperiments(was, now)

	if len(got.Cases) != 1 || got.Cases[0].ID != "a" || got.OnlyFrom != 1 {
		t.Errorf("comparison = %+v, want only a, with b left out of the older one", got)
	}
}

// Cost moves per run, so five repetitions of the same price are not five times dearer
// than one.
func TestCompareExperiments_differentRepetitions_moveCostPerRun(t *testing.T) {
	t.Parallel()
	corpus := expectsLookup("estorno")
	was := experimentOn(3, 1, repeated("estorno", 1), corpus)
	now := experimentOn(3, 5, repeated("estorno", 5), corpus)

	got := simulate.CompareExperiments(was, now)

	if got.CostMicros != 0 || changeOf(t, got, "estorno").CostMicros != 0 {
		t.Errorf("cost moved %d (case %d), want nothing: every run cost the same",
			got.CostMicros, changeOf(t, got, "estorno").CostMicros)
	}
}

// A case both versions have that one side never ran is shared, and broken there, as in a
// battery, so a regression counted is one the list shows.
func TestCompareExperiments_acrossDatasetVersions_aSharedCaseThatDidNotRun_isListedAndCounted(t *testing.T) {
	t.Parallel()
	was := experimentOn(2, 1, append(repeated("a", 1), repeated("b", 1)...), expectsLookup("a"), expectsLookup("b"))
	now := experimentOn(3, 1, repeated("a", 1), expectsLookup("a"), expectsLookup("b"), expectsLookup("c"))

	got := simulate.CompareExperiments(was, now)

	b := changeOf(t, got, "b")
	if !b.Regressed() || b.NowRan != 0 || got.Regressed != 1 || got.OnlyTo != 1 || len(got.Cases) != 2 {
		t.Errorf("comparison = %+v, want b listed as regressed with no run, c left out", got)
	}
}
