package drafts_test

import (
	"fmt"
	"testing"

	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A turn's prompt chooses its window from sizes, then reads only the turns it chose (#422):
TurnSizes measures every turn of the thread without its content, as a turn read whole would
measure it, and TurnsByID reads the chosen ones, oldest first, and nothing of another thread.
*/
func TestTurnSizesAndTurnsByID_measureEveryTurnAndReadOnlyTheChosen(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	for i := 0; i < 3; i++ {
		turn := mustBegin(t, store, agent, "usr_ana", fmt.Sprintf("k%d", i), o.Revision, 1)
		if _, err := store.SettleTurn(t.Context(), drafts.Settlement{
			Agent: agent, Caller: as("usr_ana", cx), Turn: turn.ID, Cost: 1, Reply: fmt.Sprintf("reply-%d", i),
			Validator: validator, Now: now,
		}); err != nil {
			t.Fatalf("SettleTurn(%d) = %v", i, err)
		}
	}
	whole, err := store.Turns(t.Context(), agent, as("usr_ana", cx))
	if err != nil {
		t.Fatal(err)
	}

	sizes, err := store.TurnSizes(t.Context(), agent, as("usr_ana", cx))
	if err != nil || len(sizes) != len(whole) {
		t.Fatalf("TurnSizes = %v (%v), want one per turn", sizes, err)
	}
	for i, s := range sizes {
		if s.ID != whole[i].ID || s.Bytes != len(whole[i].Message)+len(whole[i].Reply) {
			t.Errorf("size %d = %+v, want %s at %d bytes", i, s, whole[i].ID, len(whole[i].Message)+len(whole[i].Reply))
		}
	}
	chosen, err := store.TurnsByID(t.Context(), agent, as("usr_ana", cx), []string{whole[2].ID, whole[0].ID, "00000000-0000-0000-0000-000000000000"})
	if err != nil || len(chosen) != 2 || chosen[0].Reply != "reply-0" || chosen[1].Reply != "reply-2" {
		t.Errorf("TurnsByID = %+v (%v), want the first and the third, oldest first, and no stranger", chosen, err)
	}
}
