package conversation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/publishable"
	"github.com/fuseone/agents/internal/testdb"
)

/*
What a turn leaves behind is all of it or a failure (NT-018 phase 3).

A turn is done only with its reply and its proposals stored; a store that
could not judge them leaves the turn failed, at its cost, never done and
empty. A thread erased while the model was thinking stays erased. And a key
already stored answers what was stored, whatever changed in the installation
since.
*/

// unreadable is a catalogue that cannot be read.
type unreadable struct{}

func (unreadable) Tools(context.Context) ([]domain.ToolEntry, error) {
	return nil, errors.New("catalogue unavailable")
}

func TestRun_aStoreThatCannotJudgeTheProposals_failsTheTurnRatherThanFinishingIt(t *testing.T) {
	store := drafts.NewStore(testdb.Pool(t))
	agent, _ := newDraft(t, store)
	m := &scripted{reply: reply("Sugiro a busca.", draft.AddTool{Tool: "crm.lookup"}), cost: 30}
	e := engine(store, m)
	e.Validator = draft.Validator{Checker: publishable.Checker{Tools: unreadable{}}}

	if _, err := run(t, e, agent, "k1"); err == nil {
		t.Fatal("Run succeeded with proposals nobody could judge")
	}
	again, err := run(t, engine(store, m), agent, "k1")
	if err != nil || again.Turn.State != drafts.TurnFailed || again.Turn.Cost != 30 || m.calls.Load() != 1 {
		t.Fatalf("the key again = %+v (%v) after %d calls, want the turn failed at its cost, called once",
			again.Turn, err, m.calls.Load())
	}
}

func TestRun_aThreadErasedDuringTheCall_staysErased(t *testing.T) {
	store := drafts.NewStore(testdb.Pool(t))
	agent, _ := newDraft(t, store)
	m := &scripted{reply: reply("Sugiro a busca.", draft.AddTool{Tool: "crm.lookup"}), cost: 30}
	m.during = func() {
		if err := store.EraseThread(context.Background(), agent, caller()); err != nil {
			t.Errorf("EraseThread during the call: %v", err)
		}
	}

	got, err := run(t, engine(store, m), agent, "k1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	turns, _ := store.Turns(t.Context(), agent, caller())
	proposals, _ := store.Proposals(t.Context(), agent, caller())
	if len(got.Proposals) != 0 || len(proposals) != 0 || turns[0].Reply != "" || turns[0].Message != "" || turns[0].Cost != 30 {
		t.Errorf("after the erasure: result %+v, turns %+v, %d proposals; want nothing said kept, the cost kept",
			got, turns, len(proposals))
	}
}

func TestRun_aKeyAlreadyStored_answersItWhateverChangedSince(t *testing.T) {
	store := drafts.NewStore(testdb.Pool(t))
	agent, _ := newDraft(t, store)
	m := &scripted{reply: reply("Pronto."), cost: 10}
	if _, err := run(t, engine(store, m), agent, "k1"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	unpriced := engine(store, m)
	unpriced.Priced = false

	again, err := run(t, unpriced, agent, "k1")
	if err != nil || !again.Replayed || again.Turn.State != drafts.TurnDone {
		t.Errorf("the key again, unpriced since = %+v (%v), want the stored turn", again, err)
	}
}
