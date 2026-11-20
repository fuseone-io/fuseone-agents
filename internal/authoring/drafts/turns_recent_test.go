package drafts_test

import (
	"fmt"
	"testing"

	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A turn's prompt reads only the newest turns it can show (#422): asked in SQL with a limit,
answered oldest first, and the content of every older turn left unread. The full listing
of the conversation still answers every turn.
*/
func TestRecentTurns_areTheNewestOfALongerThread_oldestFirst(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	const history, window = 13, 10
	for i := 0; i < history; i++ {
		turn := mustBegin(t, store, agent, "usr_ana", fmt.Sprintf("k%02d", i), o.Revision, 1)
		if _, err := store.SettleTurn(t.Context(), drafts.Settlement{
			Agent: agent, Caller: as("usr_ana", cx), Turn: turn.ID, Cost: 1, Reply: fmt.Sprintf("r%02d", i),
			Validator: validator, Now: now,
		}); err != nil {
			t.Fatalf("SettleTurn(%d) = %v", i, err)
		}
	}

	got, err := store.RecentTurns(t.Context(), agent, as("usr_ana", cx), window)
	if err != nil {
		t.Fatalf("RecentTurns = %v", err)
	}
	if len(got) != window || got[0].Reply != "r03" || got[window-1].Reply != "r12" {
		t.Fatalf("recent = %d turns from %q to %q, want %d from r03 to r12", len(got), first(got), last(got), window)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Seq <= got[i-1].Seq {
			t.Fatalf("turn %d (seq %d) after seq %d: want oldest first", i, got[i].Seq, got[i-1].Seq)
		}
	}
	if all, err := store.Turns(t.Context(), agent, as("usr_ana", cx)); err != nil || len(all) != history {
		t.Errorf("the full listing = %d turns (%v), want all %d", len(all), err, history)
	}
}

func first(ts []drafts.Turn) string {
	if len(ts) == 0 {
		return ""
	}
	return ts[0].Reply
}

func last(ts []drafts.Turn) string {
	if len(ts) == 0 {
		return ""
	}
	return ts[len(ts)-1].Reply
}
