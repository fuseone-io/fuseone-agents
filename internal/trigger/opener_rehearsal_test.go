package trigger_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/trigger"
)

/*
The opener opens a live rehearsal on a candidate (NT-018 phase 4, release 4b): a simulated
run of a seal, marked as a rehearsal and tied to its author's conversation, so that erasing
the conversation reaches it. A rehearsal always runs a candidate, and only inside a
simulation: a rehearsal mark anywhere else is refused, never honoured.
*/

const authorThread = "7d0c3e52-8a43-4b55-9a3f-0d6a3c5f9e11"

func rehearsalOnSeal(key string) trigger.Request {
	return trigger.Request{
		Agent: "triage", IdemKey: key, Trigger: "rehearsal", Simulation: "reh:" + key,
		Case: "case-digest", Rehearsal: true, SealID: candidateSeal, AuthoringThread: authorThread,
		Input: []byte("O cliente pede o reembolso do pedido 123."),
	}
}

func TestOpen_aRehearsalOnACandidate_isMarkedAndTiedToItsAuthorsConversation(t *testing.T) {
	t.Parallel()
	opener, store := candidateOpener(t, false)

	got, err := opener.Open(t.Context(), rehearsalOnSeal("reh-1"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	steps, err := store.Read(context.Background(), got.RunID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
		t.Fatal(err)
	}
	if !started.Rehearsal || !started.Simulated || started.SealID != candidateSeal ||
		started.AuthoringThread != authorThread || started.Experiment != "" || steps[0].Scope != sealedScope {
		t.Fatalf("opened %+v in %s, want a simulated rehearsal of the seal, tied to the conversation", started, steps[0].Scope)
	}
}

func TestOpen_aRehearsalMarkOutsideACandidatesSimulation_isRefused(t *testing.T) {
	t.Parallel()
	noSeal := rehearsalOnSeal("no-seal")
	noSeal.SealID = ""
	real := rehearsalOnSeal("real")
	real.Simulation = ""
	for name, req := range map[string]trigger.Request{"no seal": noSeal, "not simulated": real} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opener, store := candidateOpener(t, false)
			if _, err := opener.Open(t.Context(), req); !errors.Is(err, trigger.ErrRehearsalOutsideCandidate) {
				t.Fatalf("err = %v, want ErrRehearsalOutsideCandidate", err)
			}
			if _, err := store.RunByIdemKey(context.Background(), req.IdemKey); err == nil {
				t.Error("a refused rehearsal opened a run")
			}
		})
	}
}
