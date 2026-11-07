package simulate

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
One case, two runs.

A pod of the previous release keys a battery's runs by position and a pod of this one by
case, so the two can open one scheduled window twice. The runs can disagree, and a battery
that judged only one could report as held a case that broke, and let an agent start.
*/

func twoRuns(first, second Case) Report {
	return Report{ID: "sim-1", Cases: []Case{first, second}}
}

func lookedUp() []Act {
	return []Act{{Tool: "crm.lookup", Verdict: domain.VerdictAllow, Reached: true}}
}

func expectsLookup(id string) []domain.RegressionCase {
	return []domain.RegressionCase{{
		ID: id, Expectations: []domain.Expectation{{Kind: domain.ExpectCalls, Value: "crm.lookup"}},
	}}
}

func TestBattery_twoRunsOfOneCaseThatDisagree_countAsBroken(t *testing.T) {
	t.Parallel()
	held := Case{ID: "estorno", Settled: SettledFinished, Acted: lookedUp()}
	broke := Case{ID: "estorno", Settled: SettledFinished}

	for name, report := range map[string]Report{
		"the broken one first": twoRuns(broke, held),
		"the broken one last":  twoRuns(held, broke),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := Battery(report, expectsLookup("estorno"))

			if got.Broken != 1 || got.Held != 0 {
				t.Errorf("broken = %d, held = %d, want it broken", got.Broken, got.Held)
			}
			if len(got.Cases) != 1 || len(got.Cases[0].Unmet) == 0 {
				t.Errorf("cases = %+v, want one row, with what it missed", got.Cases)
			}
		})
	}
}

func TestBattery_twoRunsOfOneCaseThatAgree_holdOnce(t *testing.T) {
	t.Parallel()
	held := Case{ID: "estorno", Settled: SettledFinished, Acted: lookedUp()}

	got := Battery(twoRuns(held, held), expectsLookup("estorno"))

	if got.Held != 1 || got.Broken != 0 || len(got.Cases) != 1 {
		t.Errorf("held = %d, broken = %d, rows = %d, want one held row", got.Held, got.Broken, len(got.Cases))
	}
}

// Both runs were paid for. One row for the case, and the whole bill.
func TestBattery_twoRunsOfOneCase_keepWhatBothCost(t *testing.T) {
	t.Parallel()
	report := twoRuns(
		Case{ID: "estorno", Settled: SettledFinished, Acted: lookedUp(), Cost: domain.Cost{Micros: 100}},
		Case{ID: "estorno", Settled: SettledFinished, Acted: lookedUp(), Cost: domain.Cost{Micros: 50}},
	)

	got := Battery(report, expectsLookup("estorno")).Tally()

	if got.Cases != 1 || got.Cost.Micros != 150 {
		t.Errorf("cases = %d, cost = %d, want one case and the whole bill", got.Cases, got.Cost.Micros)
	}
}

// With no corpus to read it against, Compare still sees both. A case that broke in either
// run did not hold.
func TestCompare_twoRunsOfOneCaseThatDisagree_standAsBroken(t *testing.T) {
	t.Parallel()
	unmet := []domain.Expectation{{Kind: domain.ExpectCalls}}
	broke, held := Case{ID: "estorno", Unmet: unmet}, Case{ID: "estorno"}

	for name, cases := range map[string][]Case{
		"the broken one first": {broke, held},
		"the broken one last":  {held, broke},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := Compare(reportOf("v3", Case{ID: "estorno"}), reportOf("v4", cases...))
			if got.Regressed != 1 {
				t.Errorf("regressed = %d, want the run that broke to count", got.Regressed)
			}
		})
	}
}

// Still going is not held yet. The battery has to keep saying a run is unsettled.
func TestBattery_twoRunsOfOneCase_oneStillGoing_isNotSettled(t *testing.T) {
	t.Parallel()
	done := Case{ID: "estorno", Settled: SettledFinished, Acted: lookedUp()}
	going := Case{ID: "estorno", Settled: SettledUnsettled, Acted: lookedUp()}

	for name, report := range map[string]Report{
		"going first": twoRuns(going, done),
		"going last":  twoRuns(done, going),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := Battery(report, expectsLookup("estorno")); got.Cases[0].Settled != SettledUnsettled {
				t.Errorf("settled = %s, want it still going", got.Cases[0].Settled)
			}
		})
	}
}

// Uploaded rows have no name, and two of them are two cases.
func TestBattery_twoRowsWithNoName_stayTwo(t *testing.T) {
	t.Parallel()

	got := Battery(twoRuns(Case{}, Case{}), expectsLookup("estorno"))

	// The corpus case did not run, so it is a third row.
	if len(got.Cases) != 3 {
		t.Errorf("rows = %d, want both unnamed rows and the case that did not run", len(got.Cases))
	}
}
