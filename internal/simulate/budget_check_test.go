package simulate_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

/*
An agent that reaches the right answer for three times the money.

Every assertion the corpus could make was about shape — which tool, which
ending, whether a person was asked. So a version that did all of it correctly
and spent triple passed green, and the regression nobody could express was the
one that shows up on the invoice.

Cost and steps are ceilings rather than targets: a case that got cheaper is not
a failure, and asserting an exact figure would break the battery every time a
provider changed its tokeniser.
*/
func TestCheck_caseCostsMoreThanAllowed_isBroken(t *testing.T) {
	t.Parallel()
	spent := simulate.Case{
		Settled: simulate.SettledFinished,
		Cost:    domain.Cost{Micros: 900_000},
	}

	unmet := simulate.Check(spent, []domain.Expectation{
		{Kind: domain.ExpectCostsAtMost, Value: "500000"},
	})
	if len(unmet) != 1 {
		t.Fatalf("unmet = %+v, want the ceiling reported", unmet)
	}
}

func TestCheck_caseCostsLessThanAllowed_holds(t *testing.T) {
	t.Parallel()
	// Cheaper is not a regression. A corpus that failed on an improvement is
	// one people stop running.
	cheap := simulate.Case{
		Settled: simulate.SettledFinished,
		Cost:    domain.Cost{Micros: 120_000},
	}

	if unmet := simulate.Check(cheap, []domain.Expectation{
		{Kind: domain.ExpectCostsAtMost, Value: "500000"},
	}); len(unmet) != 0 {
		t.Errorf("unmet = %+v for a case that came in under the ceiling", unmet)
	}
}

func TestCheck_caseTookMoreStepsThanAllowed_isBroken(t *testing.T) {
	t.Parallel()
	long := simulate.Case{Settled: simulate.SettledFinished, Steps: 41}

	if unmet := simulate.Check(long, []domain.Expectation{
		{Kind: domain.ExpectWithinSteps, Value: "20"},
	}); len(unmet) != 1 {
		t.Errorf("unmet = %+v, want the step ceiling reported", unmet)
	}
}

// A ceiling nothing can read is not quietly satisfied, for the same reason an
// unknown kind is not: a battery that passes what it did not check is worse
// than one that fails.
func TestCheck_ceilingIsNotANumber_isBroken(t *testing.T) {
	t.Parallel()
	any := simulate.Case{Settled: simulate.SettledFinished, Steps: 1}

	if unmet := simulate.Check(any, []domain.Expectation{
		{Kind: domain.ExpectWithinSteps, Value: "several"},
	}); len(unmet) != 1 {
		t.Errorf("unmet = %+v, want an unreadable ceiling to fail", unmet)
	}
}
