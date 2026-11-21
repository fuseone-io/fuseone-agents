package simulate

import (
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A replayed case that left its world (NT-018 phase 5) is not comparable: what it did answered
a situation nobody corrected, so it neither held nor broke. A battery counts it apart and
names the divergence, and a comparison never reads it as a regression or a fix. Across the runs
of one case, a run that followed its world and broke is still broken: that run is evidence.
*/

var anExtraCall = &domain.ReplayDivergedPayload{Position: 2, Divergence: domain.DivergenceExtra, Tool: "kb.search"}

func diverged(c Case) Case {
	c.Diverged = anExtraCall
	return c
}

func TestFold_aReplayedRunThatLeftItsWorld_namesTheDivergence(t *testing.T) {
	t.Parallel()
	payload, _ := json.Marshal(anExtraCall)
	got, err := Fold([]domain.Step{
		{Seq: 1, Kind: domain.StepRunStarted, Payload: []byte(`{"simulated":true,"case":"estorno","dataset_version":2}`)},
		{Seq: 2, Kind: domain.StepReplayDiverged, Payload: payload},
		{Seq: 3, Kind: domain.StepRunFinished, Payload: []byte(`{}`)},
	})
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	if got.Diverged == nil || *got.Diverged != *anExtraCall {
		t.Errorf("Diverged = %+v, want %+v", got.Diverged, anExtraCall)
	}
}

func TestBattery_aCaseThatLeftItsWorld_isNotComparableAndNeverHeld(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		runs []Case
		want string
	}{
		// It met what it expects, and that still says nothing.
		"one run that left its world": {[]Case{diverged(Case{ID: "estorno", Settled: SettledFinished, Acted: lookedUp()})}, "not comparable"},
		"one followed and held, one left": {[]Case{
			{ID: "estorno", Settled: SettledFinished, Acted: lookedUp(), RunID: "r1"},
			diverged(Case{ID: "estorno", Settled: SettledFinished, Acted: lookedUp(), RunID: "r2"}),
		}, "not comparable"},
		"one followed and broke, one left": {[]Case{
			{ID: "estorno", Settled: SettledFinished, RunID: "r1"},
			diverged(Case{ID: "estorno", Settled: SettledFinished, Acted: lookedUp(), RunID: "r2"}),
		}, "broken"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := Battery(Report{ID: "sim-1", Cases: tc.runs}, expectsLookup("estorno"))
			stood := map[bool]string{true: "not comparable", false: "broken"}[got.NotComparable == 1 && got.Broken == 0]
			if got.Held != 0 || stood != tc.want || got.NotComparable+got.Broken != 1 {
				t.Errorf("held %d, broken %d, not comparable %d; want %s", got.Held, got.Broken, got.NotComparable, tc.want)
			}
			if tc.want == "not comparable" && (len(got.Cases) != 1 || got.Cases[0].Diverged == nil || len(got.Cases[0].Unmet) != 0) {
				t.Errorf("row %+v, want it naming the divergence and no unmet expectation", got.Cases)
			}
		})
	}
}

func TestCompare_aCaseThatLeftItsWorld_isNeitherARegressionNorAFix(t *testing.T) {
	t.Parallel()
	before := Battery(Report{ID: "v1", Cases: []Case{{ID: "estorno", Settled: SettledFinished, Acted: lookedUp()}}}, expectsLookup("estorno"))
	after := Battery(Report{ID: "v2", Cases: []Case{diverged(Case{ID: "estorno", Settled: SettledFinished})}}, expectsLookup("estorno"))
	got := Compare(before, after)
	if len(got.Cases) != 1 || got.Cases[0].Now != NotComparable || got.Cases[0].Regressed() {
		t.Errorf("cases %+v, want the case not comparable now, and no regression", got.Cases)
	}
}

func TestReadExperiment_countsTheRepetitionsThatLeftTheWorld(t *testing.T) {
	t.Parallel()
	report := Report{ID: "exp-1", Cases: []Case{
		{ID: "estorno", Settled: SettledFinished, Acted: lookedUp(), RunID: "r1"},
		diverged(Case{ID: "estorno", Settled: SettledFinished, RunID: "r2"}),
	}}
	got := ReadExperiment(Record{}, report, expectsLookup("estorno"))
	if got.NotComparable != 1 || got.Held != 0 || len(got.Cases) != 1 {
		t.Fatalf("summary %+v, want the case counted not comparable", got.ExperimentSummary)
	}
	if c := got.Cases[0]; c.Ran != 2 || c.DivergedIn != 1 || c.BrokenIn != 0 {
		t.Errorf("ran %d, diverged in %d, broken in %d; want 2, 1, 0", c.Ran, c.DivergedIn, c.BrokenIn)
	}
}
