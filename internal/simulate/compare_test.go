package simulate

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
Is the new version better than the old one on this corpus?

The question somebody is actually asking when they publish, and the one two
separate reports cannot answer: reading them side by side leaves the reader
matching case ids by eye and getting it wrong on the twentieth row.
*/

func TestCompare_aCaseThatHeldAndNoLongerDoes_isARegression(t *testing.T) {
	t.Parallel()

	was := reportOf("v3", Case{ID: "estorno"}, Case{ID: "acesso"})
	now := reportOf("v4",
		Case{ID: "estorno", Unmet: []domain.Expectation{{Kind: domain.ExpectCalls}}},
		Case{ID: "acesso"})

	got := Compare(was, now)

	if got.Regressed != 1 || got.Fixed != 0 {
		t.Fatalf("regressed=%d fixed=%d, want one regression", got.Regressed, got.Fixed)
	}
	if got.Cases[0].ID != "estorno" {
		t.Errorf("first row = %q, want the case that changed", got.Cases[0].ID)
	}
	// Ordered so the answer is the first thing read. A comparison that lists
	// forty unchanged cases before the one that broke has hidden it.
	if got.Cases[0].Was != Held || got.Cases[0].Now != Broke {
		t.Errorf("row = %+v, want held then broken", got.Cases[0])
	}
}

func TestCompare_aCaseThatBrokeAndNowHolds_isFixed(t *testing.T) {
	t.Parallel()

	was := reportOf("v3", Case{ID: "estorno", Unmet: []domain.Expectation{{Kind: domain.ExpectCalls}}})
	now := reportOf("v4", Case{ID: "estorno"})

	got := Compare(was, now)
	if got.Fixed != 1 || got.Regressed != 0 {
		t.Errorf("fixed=%d regressed=%d, want one fixed", got.Fixed, got.Regressed)
	}
}

// The whole point of matching by id: reports are folded in the order the
// ledger holds the runs, which is not the order the corpus was written in.
func TestCompare_casesInADifferentOrder_areStillMatchedToThemselves(t *testing.T) {
	t.Parallel()

	was := reportOf("v3", Case{ID: "a"}, Case{ID: "b", Unmet: []domain.Expectation{{}}})
	now := reportOf("v4", Case{ID: "b"}, Case{ID: "a"})

	got := Compare(was, now)
	if got.Fixed != 1 || got.Regressed != 0 {
		t.Errorf("fixed=%d regressed=%d, want b fixed and a unchanged", got.Fixed, got.Regressed)
	}
}

// A case only one side ran is reported, never dropped. A comparison that
// silently ignored it would answer "nothing changed" about a corpus that grew.
func TestCompare_aCaseOnlyTheNewVersionRan_isReportedAsNew(t *testing.T) {
	t.Parallel()

	got := Compare(reportOf("v3", Case{ID: "a"}), reportOf("v4", Case{ID: "a"}, Case{ID: "b"}))

	if len(got.Cases) != 2 {
		t.Fatalf("cases = %d, want both", len(got.Cases))
	}
	var found bool
	for _, c := range got.Cases {
		if c.ID == "b" {
			found = c.Was == Absent && c.Now == Held
		}
	}
	if !found {
		t.Errorf("cases = %+v, want b as new and holding", got.Cases)
	}
}

/*
An agent that reaches the same answer for three times the money is worse, and
a comparison of held-and-broken alone reports it as identical. Money is the
reason a ceiling is worth having and the reason this is worth showing.
*/
func TestCompare_sameOutcomeForMoreMoney_saysHowMuchMore(t *testing.T) {
	t.Parallel()

	was := reportOf("v3", Case{ID: "a", Cost: domain.Cost{Micros: 1_000}, Steps: 4})
	now := reportOf("v4", Case{ID: "a", Cost: domain.Cost{Micros: 3_000}, Steps: 9})

	got := Compare(was, now)
	if got.Cases[0].CostMicros != 2_000 || got.Cases[0].Steps != 5 {
		t.Errorf("row = %+v, want the rise stated", got.Cases[0])
	}
	if got.Regressed != 0 {
		t.Error("a rise in cost was counted as a broken correction")
	}
}

func reportOf(version domain.VersionID, cases ...Case) Report {
	return Report{Version: version, Cases: cases}
}

// Read on a screen an approver was shown and in an export somebody keeps. The
// rows are built from maps, so a comparison that did not order them fully
// would answer the same question differently on a second reading.
func TestCompare_readTwice_answersInTheSameOrder(t *testing.T) {
	t.Parallel()

	was := reportOf("v3", Case{ID: "c"}, Case{ID: "a"}, Case{ID: "b"}, Case{ID: "d"})
	now := reportOf("v4", Case{ID: "a"}, Case{ID: "d"}, Case{ID: "b"}, Case{ID: "c"})

	first := Compare(was, now)
	for range 20 {
		again := Compare(was, now)
		for at := range first.Cases {
			if first.Cases[at].ID != again.Cases[at].ID {
				t.Fatalf("row %d = %q then %q", at, first.Cases[at].ID, again.Cases[at].ID)
			}
		}
	}
}
