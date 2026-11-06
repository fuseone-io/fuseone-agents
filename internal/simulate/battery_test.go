package simulate_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

// A battery is the corpus run again. What it must get right is which
// expectation belongs to which case: checking case three against case one's
// correction reports a failure nobody can act on and hides a real one.

func corpusCase(id string, e ...domain.Expectation) domain.RegressionCase {
	return domain.RegressionCase{ID: id, Agent: "suporte", Expectations: e}
}

func TestBattery_matchesACaseToItsOwnCorrection(t *testing.T) {
	t.Parallel()

	report := simulate.Report{Cases: []simulate.Case{
		// Deliberately not in corpus order: runs are folded in the order the
		// ledger holds them, which is not the order a corpus was written in.
		{ID: "reg-2", RunID: "run-b", Settled: simulate.SettledFinished,
			Acted: []simulate.Act{acted("Responder", "crm.refund", domain.VerdictAllow, true)}},
		{ID: "reg-1", RunID: "run-a", Settled: simulate.SettledWaiting,
			Acted: []simulate.Act{acted("Responder", "email.send", domain.VerdictRequireApproval, false)}},
	}}

	got := simulate.Battery(report, []domain.RegressionCase{
		corpusCase("reg-1", domain.Expectation{Kind: domain.ExpectAsks}),
		corpusCase("reg-2", domain.Expectation{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}),
	})

	byID := map[string]simulate.Case{}
	for _, c := range got.Cases {
		byID[c.ID] = c
	}
	if len(byID["reg-1"].Unmet) != 0 {
		t.Errorf("reg-1 unmet = %+v, want none", byID["reg-1"].Unmet)
	}
	if len(byID["reg-2"].Unmet) != 1 {
		t.Errorf("reg-2 unmet = %+v, want the refund correction broken", byID["reg-2"].Unmet)
	}
}

func TestBattery_aCorpusCaseThatDidNotRun_isReportedAsBroken(t *testing.T) {
	t.Parallel()

	// The corpus is what the battery promises to check. A case missing from
	// the report is a case nobody checked, and reporting only what ran would
	// make the promise quietly smaller than it says.
	got := simulate.Battery(simulate.Report{}, []domain.RegressionCase{
		corpusCase("reg-1", domain.Expectation{Kind: domain.ExpectSettles, Value: "finished"}),
	})

	if len(got.Cases) != 1 {
		t.Fatalf("cases = %+v, want the one that never ran", got.Cases)
	}
	if len(got.Cases[0].Unmet) != 1 || got.Cases[0].Error == "" {
		t.Errorf("case = %+v, want it counted as unmet and said why", got.Cases[0])
	}
}

func TestBattery_holdsReportsHowManyCorrectionsStillStand(t *testing.T) {
	t.Parallel()

	report := simulate.Report{Cases: []simulate.Case{
		{ID: "reg-1", RunID: "run-a", Settled: simulate.SettledFinished},
		{ID: "reg-2", RunID: "run-b", Settled: simulate.SettledParked},
	}}

	got := simulate.Battery(report, []domain.RegressionCase{
		corpusCase("reg-1", domain.Expectation{Kind: domain.ExpectSettles, Value: "finished"}),
		corpusCase("reg-2", domain.Expectation{Kind: domain.ExpectSettles, Value: "finished"}),
	})

	// The one number an author reads before anything else: is what I already
	// fixed still fixed?
	if got.Held != 1 || got.Broken != 1 {
		t.Errorf("held %d broken %d, want one of each", got.Held, got.Broken)
	}
}

func TestBattery_withoutACorpus_changesNothing(t *testing.T) {
	t.Parallel()

	// An ordinary simulation of an uploaded set is not a battery, and must not
	// grow failures out of a corpus that has nothing to say about it.
	report := simulate.Report{Cases: []simulate.Case{{RunID: "run-a", Settled: simulate.SettledFinished}}}
	got := simulate.Battery(report, nil)

	if got.Broken != 0 || len(got.Cases[0].Unmet) != 0 {
		t.Errorf("report = %+v", got)
	}
}

// A case whose input was erased cannot run again. It is neither a correction
// that holds nor one that broke, and calling it either would be a false claim
// about an agent: it is a gap, and the report says so.
func TestBattery_aCaseWhoseInputWasErased_isAGapNotABreak(t *testing.T) {
	t.Parallel()

	erased := corpusCase("gone", domain.Expectation{Kind: domain.ExpectAsks})
	erased.Erased = true
	report := simulate.Battery(simulate.Report{}, []domain.RegressionCase{erased})

	if report.Erased != 1 || report.Broken != 0 || report.Held != 0 {
		t.Fatalf("erased=%d broken=%d held=%d, want a single gap",
			report.Erased, report.Broken, report.Held)
	}
	if len(report.Cases) != 1 || report.Cases[0].Settled != simulate.SettledErased || len(report.Cases[0].Unmet) != 0 {
		t.Fatalf("row = %+v", report.Cases)
	}
}

// Erased after it ran. The run happened and is still a fact, so a past battery
// keeps saying what that run did.
func TestBattery_aCaseErasedAfterItRan_isStillJudgedOnThatRun(t *testing.T) {
	t.Parallel()

	erased := corpusCase("gone", domain.Expectation{Kind: domain.ExpectSettles, Value: "finished"})
	erased.Erased = true
	report := simulate.Battery(simulate.Report{Cases: []simulate.Case{
		{ID: "gone", RunID: "run-1", Settled: simulate.SettledFinished},
	}}, []domain.RegressionCase{erased})

	if report.Held != 1 || report.Erased != 0 {
		t.Fatalf("held=%d erased=%d, want it judged on the run", report.Held, report.Erased)
	}
}

func TestCompare_aCaseErasedOnOneSide_isNeitherFixedNorRegressed(t *testing.T) {
	t.Parallel()

	erased := corpusCase("gone", domain.Expectation{Kind: domain.ExpectSettles, Value: "finished"})
	erased.Erased = true
	ran := simulate.Battery(simulate.Report{Cases: []simulate.Case{
		{ID: "gone", RunID: "run-1", Settled: simulate.SettledParked},
	}}, []domain.RegressionCase{erased})
	gap := simulate.Battery(simulate.Report{}, []domain.RegressionCase{erased})

	for name, compared := range map[string]simulate.Comparison{
		"erased since":  simulate.Compare(ran, gap),
		"erased before": simulate.Compare(gap, ran),
	} {
		if compared.Regressed != 0 || compared.Fixed != 0 {
			t.Errorf("%s: regressed=%d fixed=%d", name, compared.Regressed, compared.Fixed)
		}
	}
}

func TestTally_aCaseThatLostItsInput_isNotStillGoing(t *testing.T) {
	t.Parallel()

	erased := corpusCase("gone", domain.Expectation{Kind: domain.ExpectAsks})
	erased.Erased = true
	tally := simulate.Battery(simulate.Report{}, []domain.RegressionCase{erased}).Tally()

	if tally.Unsettled != 0 {
		t.Fatalf("unsettled = %d, want none: nothing is running", tally.Unsettled)
	}
}
