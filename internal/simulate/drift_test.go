package simulate_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

/*
Telling a change from a drift.

A rising broken count has two very different causes: somebody changed the
agent, or a provider changed the model under a name that did not change. The
second is the one nobody notices, because nothing in this installation moved —
and in an installation inside somebody else's network, nobody notices until it
is an incident.

So a case records the model it last held against, and the battery says when
that is no longer the model answering. It is not a pass and not a failure: it
is the reason to read the rest of the row differently.
*/
func heldAgainst(model string, unmet bool) (simulate.Report, []domain.RegressionCase) {
	settled := simulate.SettledFinished
	if unmet {
		settled = simulate.SettledParked
	}
	report := simulate.Report{Cases: []simulate.Case{{
		ID: "case-1", RunID: "run-1", Settled: settled, Model: "claude-sonnet-5",
	}}}
	corpus := []domain.RegressionCase{{
		ID:    "case-1",
		Model: model,
		Expectations: []domain.Expectation{
			{Kind: domain.ExpectSettles, Value: string(simulate.SettledFinished)},
		},
	}}
	return report, corpus
}

func TestBattery_theModelMoved_isCountedAsDrift(t *testing.T) {
	t.Parallel()
	report, corpus := heldAgainst("claude-sonnet-4-6", false)

	out := simulate.Battery(report, corpus)
	if out.Drifted != 1 {
		t.Errorf("drifted = %d, want the moved model reported", out.Drifted)
	}
	// Still held. A case that passes against a model that moved is worth
	// knowing about too: it is the one that will break next.
	if out.Held != 1 || out.Broken != 0 {
		t.Errorf("held=%d broken=%d, want it still holding", out.Held, out.Broken)
	}
}

func TestBattery_sameModel_isNotDrift(t *testing.T) {
	t.Parallel()
	report, corpus := heldAgainst("claude-sonnet-5", true)

	out := simulate.Battery(report, corpus)
	if out.Drifted != 0 {
		t.Errorf("drifted = %d for a case answered by the model it always was", out.Drifted)
	}
	if out.Broken != 1 {
		t.Errorf("broken = %d, want the failure still reported", out.Broken)
	}
}

// A corpus recorded before any of this knows no model. Reporting every one of
// those as drift would make the count useless on the day it is switched on.
func TestBattery_caseFromBeforeTheModelWasRecorded_isNotDrift(t *testing.T) {
	t.Parallel()
	report, corpus := heldAgainst("", false)

	if out := simulate.Battery(report, corpus); out.Drifted != 0 {
		t.Errorf("drifted = %d for a case that never recorded a model", out.Drifted)
	}
}
