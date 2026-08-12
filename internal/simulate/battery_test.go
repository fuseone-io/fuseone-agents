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
