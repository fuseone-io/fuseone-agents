package simulate_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

// A correction is only a regression case if something can check it without a
// person reading the run again. Everything here is about that being true.

func acted(step, tool string, verdict domain.Verdict, reached bool) simulate.Act {
	return simulate.Act{
		Step: step, Tool: domain.ToolID(tool), Effect: domain.EffectWrite,
		Verdict: verdict, Reached: reached,
	}
}

func TestCheck_neverCalls_isUnmetWhenItWouldHaveCalled(t *testing.T) {
	t.Parallel()

	got := simulate.Check(simulate.Case{
		Settled: simulate.SettledFinished,
		Acted:   []simulate.Act{acted("Responder", "crm.refund", domain.VerdictAllow, true)},
	}, []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}})

	// The correction that is worth the whole mechanism: never do this to a
	// customer, and prove it on every version from now on.
	if len(got) != 1 {
		t.Fatalf("unmet = %+v, want the refund expectation", got)
	}
}

func TestCheck_neverCalls_isMetWhenTheGateStoppedIt(t *testing.T) {
	t.Parallel()

	// Proposed and refused is not called. The author asked that it not happen,
	// and it did not happen — how it was prevented is the platform's business.
	got := simulate.Check(simulate.Case{
		Settled: simulate.SettledParked,
		Acted:   []simulate.Act{acted("Responder", "crm.refund", domain.VerdictBlock, false)},
	}, []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}})

	if len(got) != 0 {
		t.Errorf("unmet = %+v, want none", got)
	}
}

func TestCheck_anchoredToAStep_ignoresTheSameToolElsewhere(t *testing.T) {
	t.Parallel()

	// A correction about the reply step must not start failing because the
	// lookup step calls the same tool (FU-13).
	got := simulate.Check(simulate.Case{
		Settled: simulate.SettledFinished,
		Acted: []simulate.Act{
			acted("Consultar", "crm.lookup", domain.VerdictAllow, true),
			acted("Responder", "email.send", domain.VerdictAllow, true),
		},
	}, []domain.Expectation{
		{Kind: domain.ExpectNeverCalls, Step: "Responder", Value: "crm.lookup"},
	})

	if len(got) != 0 {
		t.Errorf("unmet = %+v, want none: the call was in another step", got)
	}
}

func TestCheck_asks_isMetByAnyEscalationWhenNoToolIsNamed(t *testing.T) {
	t.Parallel()

	got := simulate.Check(simulate.Case{
		Settled: simulate.SettledWaiting,
		Acted:   []simulate.Act{acted("Responder", "email.send", domain.VerdictRequireApproval, false)},
	}, []domain.Expectation{{Kind: domain.ExpectAsks}})

	if len(got) != 0 {
		t.Errorf("unmet = %+v, want none", got)
	}
}

func TestCheck_asks_isUnmetWhenItDecidedAlone(t *testing.T) {
	t.Parallel()

	// "It should have asked me" is the sentence this exists to make checkable.
	got := simulate.Check(simulate.Case{
		Settled: simulate.SettledFinished,
		Acted:   []simulate.Act{acted("Responder", "email.send", domain.VerdictAllow, true)},
	}, []domain.Expectation{{Kind: domain.ExpectAsks, Value: "email.send"}})

	if len(got) != 1 {
		t.Fatalf("unmet = %+v, want the escalation expectation", got)
	}
}

func TestCheck_calls_isUnmetWhenTheGateStoppedIt(t *testing.T) {
	t.Parallel()

	// The mirror of never_calls: an author who says it must do this is not
	// served by a proposal the policy refused.
	got := simulate.Check(simulate.Case{
		Settled: simulate.SettledParked,
		Acted:   []simulate.Act{acted("Consultar", "crm.lookup", domain.VerdictBlock, false)},
	}, []domain.Expectation{{Kind: domain.ExpectCalls, Value: "crm.lookup"}})

	if len(got) != 1 {
		t.Errorf("unmet = %+v, want the call expectation", got)
	}
}

func TestCheck_settles_comparesWhereTheCaseEnded(t *testing.T) {
	t.Parallel()

	unmet := simulate.Check(simulate.Case{Settled: simulate.SettledParked},
		[]domain.Expectation{{Kind: domain.ExpectSettles, Value: "finished"}})
	if len(unmet) != 1 {
		t.Errorf("unmet = %+v, want the settle expectation", unmet)
	}

	met := simulate.Check(simulate.Case{Settled: simulate.SettledWaiting},
		[]domain.Expectation{{Kind: domain.ExpectSettles, Value: "awaiting_approval"}})
	if len(met) != 0 {
		t.Errorf("unmet = %+v, want none", met)
	}
}

func TestCheck_aCaseThatNeverRan_failsEveryExpectation(t *testing.T) {
	t.Parallel()

	// Counting a case that could not open as passing would make a green
	// battery mean "nothing was checked" on the day it matters most.
	got := simulate.Check(simulate.Case{Error: "the agent is paused"}, []domain.Expectation{
		{Kind: domain.ExpectNeverCalls, Value: "crm.refund"},
		{Kind: domain.ExpectSettles, Value: "finished"},
	})

	if len(got) != 2 {
		t.Errorf("unmet = %d, want every expectation unmet", len(got))
	}
}
