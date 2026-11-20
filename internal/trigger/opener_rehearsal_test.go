package trigger_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

/*
The opener opens a live rehearsal on a candidate (NT-018 phase 4, release 4b): a simulated
run of a seal, marked as a rehearsal and tied to its author's conversation on the draft the
seal came from, so that erasing the conversation reaches it.

The opener resolves that conversation itself, after every refusal and before anything is
written: no caller can name a thread, a refused rehearsal opens none, and a rehearsal
without one does not open. A rehearsal is never an experiment, and only a simulation of a
candidate is one.
*/

const authorThread = "7d0c3e52-8a43-4b55-9a3f-0d6a3c5f9e11"

// threads answers a conversation for a draft and an author, and remembers who asked.
type threads struct {
	mu     sync.Mutex
	asked  []string
	answer string
	err    error
}

func (t *threads) ThreadFor(_ context.Context, agent domain.AgentID, draft string, author domain.UserID) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.asked = append(t.asked, string(agent)+"/"+draft+"/"+string(author))
	return t.answer, t.err
}

func (t *threads) calls() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.asked...)
}

func rehearsalOnSeal(key string) trigger.Request {
	return trigger.Request{
		Agent: "triage", IdemKey: key, Trigger: "rehearsal", Simulation: "reh:" + key, By: "usr_ana",
		Case: "case-digest", Rehearsal: true, SealID: candidateSeal,
		Input: []byte("O cliente pede o reembolso do pedido 123."),
	}
}

func rehearsalOpener(t *testing.T, paused bool) (*trigger.Opener, *ledger.Memory, *threads) {
	t.Helper()
	opener, store := candidateOpener(t, paused)
	conversations := &threads{answer: authorThread}
	return opener.WithThreads(conversations), store, conversations
}

func TestOpen_aRehearsalOnACandidate_isMarkedAndTiedToItsAuthorsConversation(t *testing.T) {
	t.Parallel()
	opener, store, conversations := rehearsalOpener(t, false)

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
	if got := conversations.calls(); len(got) != 1 || got[0] != "triage/"+candidateDraft+"/usr_ana" {
		t.Errorf("the conversation was asked as %v, want the seal's draft and the caller", got)
	}
}

func TestOpen_aRehearsalMarkOutsideACandidatesSimulation_isRefused(t *testing.T) {
	t.Parallel()
	noSeal := rehearsalOnSeal("no-seal")
	noSeal.SealID = ""
	real := rehearsalOnSeal("real")
	real.Simulation = ""
	experiment := rehearsalOnSeal("experiment")
	experiment.Experiment = "exp_triage_1"
	nobody := rehearsalOnSeal("nobody")
	nobody.By = ""
	for name, req := range map[string]trigger.Request{
		"no seal": noSeal, "not simulated": real, "an experiment as well": experiment, "no author": nobody,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opener, store, conversations := rehearsalOpener(t, false)
			if _, err := opener.Open(t.Context(), req); !errors.Is(err, trigger.ErrRehearsalOutsideCandidate) {
				t.Fatalf("err = %v, want ErrRehearsalOutsideCandidate", err)
			}
			if _, err := store.RunByIdemKey(context.Background(), req.IdemKey); err == nil {
				t.Error("a refused rehearsal opened a run")
			}
			if got := conversations.calls(); len(got) != 0 {
				t.Errorf("a refused rehearsal opened a conversation: %v", got)
			}
		})
	}
}

// Every refusal comes before the conversation is resolved: resolving it opens the author's
// thread the first time, and a rehearsal that does not open must leave nothing written.
func TestOpen_aRehearsalTheOpenerRefuses_opensNoConversation(t *testing.T) {
	t.Parallel()
	unknown := rehearsalOnSeal("unknown")
	unknown.SealID = "9f9f9f9f-8a43-4b55-9a3f-0d6a3c5f9e11"
	cases := map[string]struct {
		req    trigger.Request
		paused bool
		stops  stops
	}{
		"a paused agent":    {rehearsalOnSeal("paused"), true, nil},
		"a stopped scope":   {rehearsalOnSeal("stopped"), false, stops{{Level: domain.StopScope, Scope: sealedScope, Reason: "halt"}}},
		"a seal nobody has": {unknown, false, nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opener, _, conversations := rehearsalOpener(t, c.paused)
			if c.stops != nil {
				opener = opener.WithStops(c.stops)
			}
			if _, err := opener.WithPauses(paused{}).Open(t.Context(), c.req); err == nil {
				t.Fatal("the rehearsal opened")
			}
			if got := conversations.calls(); len(got) != 0 {
				t.Errorf("a refused rehearsal opened a conversation: %v", got)
			}
		})
	}
}

func TestOpen_aRehearsalWithoutItsConversation_doesNotOpen(t *testing.T) {
	t.Parallel()
	for name, conversations := range map[string]*threads{
		"no thread":         {answer: ""},
		"not a thread id":   {answer: "thread one"},
		"the store refused": {err: errors.New("drafts are down")},
		"no threads wired":  nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opener, store := candidateOpener(t, false)
			if conversations != nil {
				opener = opener.WithThreads(conversations)
			}
			if _, err := opener.Open(t.Context(), rehearsalOnSeal("no-thread")); err == nil {
				t.Fatal("a rehearsal opened without its conversation")
			}
			if _, err := store.RunByIdemKey(context.Background(), "no-thread"); err == nil {
				t.Error("a run was written")
			}
		})
	}
}

// Only a rehearsal has a conversation: an experiment on the same seal is never tied to one.
func TestOpen_anExperimentOnACandidate_hasNoConversation(t *testing.T) {
	t.Parallel()
	opener, store, conversations := rehearsalOpener(t, false)
	got, err := opener.Open(t.Context(), experimentOnSeal("exp-no-thread"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	steps, err := store.Read(context.Background(), got.RunID, domain.FirstSeq)
	if err != nil {
		t.Fatal(err)
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
		t.Fatal(err)
	}
	if started.AuthoringThread != "" || started.Rehearsal || len(conversations.calls()) != 0 {
		t.Errorf("an experiment was tied to %q (asked %v)", started.AuthoringThread, conversations.calls())
	}
}
