package simulate

import (
	"fmt"
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

// The whole bill is the total's. An expectation is about one run, and two cheap runs
// are not one expensive run.
func TestBattery_twoCheapRuns_doNotBreakAPerRunCeiling(t *testing.T) {
	t.Parallel()
	run := Case{ID: "estorno", RunID: "run-1", Settled: SettledFinished, Cost: domain.Cost{Micros: 60}}
	corpus := []domain.RegressionCase{{
		ID: "estorno", Expectations: []domain.Expectation{{Kind: domain.ExpectCostsAtMost, Value: "100"}},
	}}

	got := Battery(twoRuns(run, run), corpus)

	if got.Held != 1 || got.Broken != 0 {
		t.Errorf("held = %d, broken = %d, want it held: each run was under the ceiling", got.Held, got.Broken)
	}
	if bill := got.Tally().Cost.Micros; bill != 120 {
		t.Errorf("cost = %d, want both runs in the total", bill)
	}
}

// Whichever way the ledger holds them, one battery reads one way. Each pair is two runs
// that both meet the expectation and end differently, or carry a signal only one has.
func TestBattery_twoRunsOfOneCase_readTheSameInEitherOrder(t *testing.T) {
	t.Parallel()
	never := []domain.RegressionCase{{
		ID: "estorno", Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}}
	id := 0
	run := func(settled Settled, acted ...Act) Case {
		id++
		return Case{ID: "estorno", RunID: domain.RunID(fmt.Sprintf("run-%d", id)), Settled: settled, Acted: acted}
	}
	blocked := Act{Tool: "crm.lookup", Verdict: domain.VerdictBlock}
	pairs := map[string][2]Case{
		"finished and parked":  {run(SettledFinished), run(SettledParked)},
		"finished and waiting": {run(SettledFinished), run(SettledWaiting)},
		"parked and waiting":   {run(SettledParked), run(SettledWaiting)},
		"a block only in one":  {run(SettledFinished), run(SettledFinished, blocked)},
		"a block in the milder ending": {
			run(SettledFinished, blocked), run(SettledParked),
		},
	}
	for name, pair := range pairs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			one := Battery(twoRuns(pair[0], pair[1]), never)
			other := Battery(twoRuns(pair[1], pair[0]), never)

			if a, b := one.Tally(), other.Tally(); a != b {
				t.Errorf("tally moved with the order:\n%+v\n%+v", a, b)
			}
			if a, b := one.Cases[0].Settled, other.Cases[0].Settled; a != b {
				t.Errorf("settled moved with the order: %s and %s", a, b)
			}
		})
	}
}

// The milder ending is what a reader must not be told about: parked outweighs finished,
// so a report showing finished would read as done with nobody needing to look.
func TestBattery_oneRunParked_readsAsParked(t *testing.T) {
	t.Parallel()
	done := Case{ID: "estorno", RunID: "run-1", Settled: SettledFinished}
	parked := Case{ID: "estorno", RunID: "run-2", Settled: SettledParked}
	never := []domain.RegressionCase{{
		ID: "estorno", Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}}

	if got := Battery(twoRuns(done, parked), never).Tally(); got.Parked != 1 || got.Finished != 0 {
		t.Errorf("tally = %+v, want it parked", got)
	}
}

// A block in a run not chosen still has to reach the line an author reads first.
func TestBattery_aBlockInTheOtherRun_stillCountsAsStopped(t *testing.T) {
	t.Parallel()
	never := []domain.RegressionCase{{
		ID: "estorno", Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}}
	blockedFinished := Case{
		ID: "estorno", RunID: "run-1", Settled: SettledFinished,
		Acted: []Act{{Tool: "crm.lookup", Verdict: domain.VerdictBlock}},
	}
	parked := Case{ID: "estorno", RunID: "run-2", Settled: SettledParked}

	if got := Battery(twoRuns(blockedFinished, parked), never).Tally(); got.Stopped != 1 || got.Parked != 1 {
		t.Errorf("tally = %+v, want it parked and stopped", got)
	}
}

// One run on another model is a provider moving under a name, even if the row shown
// is the other run.
func TestBattery_aRunOnAnotherModel_stillCountsAsDrifted(t *testing.T) {
	t.Parallel()
	corpus := []domain.RegressionCase{{
		ID: "estorno", Model: "model-a",
		Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}}
	same := Case{ID: "estorno", RunID: "run-1", Settled: SettledFinished, Model: "model-a"}
	moved := Case{ID: "estorno", RunID: "run-2", Settled: SettledFinished, Model: "model-b"}

	if got := Battery(twoRuns(same, moved), corpus); got.Drifted != 1 {
		t.Errorf("drifted = %d, want it counted", got.Drifted)
	}
}

// Comparing on the row's cost, a window with two runs does not read as a price rise.
func TestCompare_twoRunsOfOneCase_moveByOneRunsCost(t *testing.T) {
	t.Parallel()
	never := []domain.RegressionCase{{
		ID: "estorno", Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}}
	one := Case{ID: "estorno", Settled: SettledFinished, Cost: domain.Cost{Micros: 60}}

	was := Battery(Report{Cases: []Case{one}}, never)
	now := Battery(twoRuns(one, one), never)

	if got := Compare(was, now).CostMicros; got != 0 {
		t.Errorf("cost movement = %d, want none", got)
	}
}

// What a reader of the row sees: the whole bill, and everything any run did.
func TestBattery_theRowOfTwoRuns_showsWhatBothDid(t *testing.T) {
	t.Parallel()
	never := []domain.RegressionCase{{
		ID: "estorno", Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}}
	blocked := Case{
		ID: "estorno", RunID: "run-1", Settled: SettledFinished, Cost: domain.Cost{Micros: 60},
		Acted: []Act{{Tool: "crm.lookup", Verdict: domain.VerdictBlock}},
	}
	parked := Case{ID: "estorno", RunID: "run-2", Settled: SettledParked, Cost: domain.Cost{Micros: 40}}

	row := Battery(twoRuns(blocked, parked), never).Cases[0]

	if row.Spent().Micros != 100 || row.Cost.Micros != 40 {
		t.Errorf("spent = %d, cost = %d, want both in what was spent", row.Spent().Micros, row.Cost.Micros)
	}
	if acts := row.EveryAct(); len(acts) != 1 || acts[0].Verdict != domain.VerdictBlock {
		t.Errorf("acts = %+v, want the block", acts)
	}
}

func TestBattery_twoRunsOfOneCase_theRowIsTheOneThatEndedWorse(t *testing.T) {
	t.Parallel()
	never := []domain.RegressionCase{{
		ID: "estorno", Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}}
	cases := []struct {
		name          string
		first, second Settled
		want          Settled
	}{
		{"waiting over finished", SettledFinished, SettledWaiting, SettledWaiting},
		{"parked over waiting", SettledWaiting, SettledParked, SettledParked},
		{"unsettled over parked", SettledParked, SettledUnsettled, SettledUnsettled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := Case{ID: "estorno", RunID: "run-1", Settled: tc.first}
			b := Case{ID: "estorno", RunID: "run-2", Settled: tc.second}
			for _, report := range []Report{twoRuns(a, b), twoRuns(b, a)} {
				if got := Battery(report, never).Cases[0].Settled; got != tc.want {
					t.Errorf("settled = %s, want %s", got, tc.want)
				}
			}
		})
	}
}

// Two runs that weigh the same, read either way: the same row, with the same model and
// the same cost, so the ledger's order decides nothing a comparison reads.
func TestBattery_twoRunsThatWeighTheSame_chooseTheSameRow(t *testing.T) {
	t.Parallel()
	never := []domain.RegressionCase{{
		ID: "estorno", Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}}
	a := Case{ID: "estorno", RunID: "run-1", Settled: SettledFinished, Cost: domain.Cost{Micros: 10}}
	b := Case{ID: "estorno", RunID: "run-2", Settled: SettledFinished, Cost: domain.Cost{Micros: 90}}

	one := Battery(twoRuns(a, b), never).Cases[0]
	other := Battery(twoRuns(b, a), never).Cases[0]

	if one.RunID != "run-1" || other.RunID != "run-1" {
		t.Errorf("rows = %s and %s, want the same one", one.RunID, other.RunID)
	}
}
