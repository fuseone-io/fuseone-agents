package model

import "testing"

/*
What a finished run says, and what it says stopped it.

The marker is a convention the model is told about in as many words. Its
absence means nothing was asserted, and nothing is ever inferred from the
prose: a trail that decided a summary "sounds like" the author's exception
would be recording a claim nobody made.
*/

func TestReadOutcome_theMarker_separatesTheExceptionFromTheSummary(t *testing.T) {
	t.Parallel()

	outcome, stopped := readOutcome(
		"STOP: não encontrar o cliente\nProcurei pelos dois e-mails e não achei.")

	if stopped != "não encontrar o cliente" {
		t.Errorf("stopped by %q, want the author's words", stopped)
	}
	if outcome != "Procurei pelos dois e-mails e não achei." {
		t.Errorf("outcome = %q, want the rest of what it said", outcome)
	}
}

func TestReadOutcome_anOrdinaryEnding_assertsNothing(t *testing.T) {
	t.Parallel()

	outcome, stopped := readOutcome("  Respondi o cliente e encerrei.  ")

	if stopped != "" {
		t.Errorf("stopped by %q, want nothing asserted", stopped)
	}
	if outcome != "Respondi o cliente e encerrei." {
		t.Errorf("outcome = %q", outcome)
	}
}

// A summary that merely mentions stopping is not the assertion. Only the
// marker is, because only the marker was asked for.
func TestReadOutcome_prosePlaceOfTheMarker_isJustProse(t *testing.T) {
	t.Parallel()

	if _, stopped := readOutcome("Parei porque não encontrei o cliente."); stopped != "" {
		t.Errorf("stopped by %q, want nothing asserted", stopped)
	}
}
