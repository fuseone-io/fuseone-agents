package drafts_test

import (
	"testing"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/testdb"
)

/*
What an answer carried that never became a card is kept on its turn (#413 review): the
operations discarded on the way in, and those made against a draft that moved during the
call. Counted on the first answer alone, a retry of the same key after a lost response, or
a reload, showed a reply announcing changes with no card and no word of why.
*/
func TestSettleTurn_whatWasDiscarded_isKeptOnTheTurn_forItsReplayAndItsListing(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	turn := mustBegin(t, store, agent, "usr_ana", "discarded", o.Revision, 100)
	applyControl(t, store, agent, o.Revision, draft.SetReplyLocale{Locale: "en-US"})
	if _, err := store.SettleTurn(t.Context(), drafts.Settlement{
		Agent: agent, Caller: as("usr_ana", cx), Turn: turn.ID, Cost: 10, Reply: "Proponho três.",
		Dropped:   2,
		Proposed:  []drafts.Proposed{{Op: draft.AddTool{Tool: "crm.lookup"}}},
		Validator: validator, Now: now,
	}); err != nil {
		t.Fatalf("SettleTurn = %v", err)
	}

	replayed, found, err := store.ReplayTurn(t.Context(), drafts.Replaying{Agent: agent, Key: "discarded", Caller: as("usr_ana", cx)})
	if err != nil || !found || replayed.Turn.Dropped != 2 || replayed.Turn.Stale != 1 {
		t.Errorf("replayed = %+v (%v, found %v), want 2 dropped and 1 stale", replayed.Turn, err, found)
	}
	listed, err := store.Turns(t.Context(), agent, as("usr_ana", cx))
	if err != nil || len(listed) != 1 || listed[0].Dropped != 2 || listed[0].Stale != 1 {
		t.Errorf("listed = %+v (%v), want the same counts", listed, err)
	}
}
