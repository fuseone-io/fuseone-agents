package simulate_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

/*
The order things happened in.

Every assertion the corpus had was about a set: which tools were reached, where
it ended, how much it cost. None could say **before**, and "it replied without
looking the customer up first" is one of the corrections people actually make —
the agent did both of the right things in the wrong order, so every existing
assertion holds and the run was still wrong.

The value is "first,second", because an ordering is about two things and one of
them alone is already expressible with `calls`.
*/
func reaching(tools ...string) simulate.Case {
	acts := make([]simulate.Act, 0, len(tools))
	for _, tool := range tools {
		acts = append(acts, simulate.Act{Tool: domain.ToolID(tool), Reached: true})
	}
	return simulate.Case{Settled: simulate.SettledFinished, Acted: acts}
}

func TestCheck_toolsReachedInOrder_holds(t *testing.T) {
	t.Parallel()
	run := reaching("crm.lookup", "crm.reply")

	if unmet := simulate.Check(run, []domain.Expectation{
		{Kind: domain.ExpectCallsBefore, Value: "crm.lookup,crm.reply"},
	}); len(unmet) != 0 {
		t.Errorf("unmet = %+v for a run that did them in order", unmet)
	}
}

func TestCheck_toolsReachedInTheWrongOrder_isBroken(t *testing.T) {
	t.Parallel()
	// Both tools reached, both `calls` assertions would hold, and the run is
	// wrong. This is the whole reason the kind exists.
	run := reaching("crm.reply", "crm.lookup")

	if unmet := simulate.Check(run, []domain.Expectation{
		{Kind: domain.ExpectCallsBefore, Value: "crm.lookup,crm.reply"},
	}); len(unmet) != 1 {
		t.Errorf("unmet = %+v, want the ordering reported", unmet)
	}
}

func TestCheck_theSecondToolNeverRan_isBroken(t *testing.T) {
	t.Parallel()
	// "Look up before replying" is not satisfied by never replying. An
	// ordering that passed when half of it did not happen would let a run that
	// did nothing satisfy an assertion about what it did.
	if unmet := simulate.Check(reaching("crm.lookup"), []domain.Expectation{
		{Kind: domain.ExpectCallsBefore, Value: "crm.lookup,crm.reply"},
	}); len(unmet) != 1 {
		t.Errorf("unmet = %+v, want a half-run ordering to fail", unmet)
	}
}

func TestCheck_orderingNamesOneTool_isBroken(t *testing.T) {
	t.Parallel()
	// A malformed expectation fails rather than passing, like every other
	// unreadable one.
	if unmet := simulate.Check(reaching("crm.lookup"), []domain.Expectation{
		{Kind: domain.ExpectCallsBefore, Value: "crm.lookup"},
	}); len(unmet) != 1 {
		t.Errorf("unmet = %+v, want an unreadable ordering to fail", unmet)
	}
}
