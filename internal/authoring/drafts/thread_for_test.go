package drafts_test

import (
	"testing"

	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The author's conversation on a draft, for a rehearsal to belong to (NT-018 phase 4): the
same thread the author's turns and proposals use, opened the first time it is asked for,
and never another author's.
*/
func TestThreadFor_isTheAuthorsOwnThreadOnTheDraft_openedOnce(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)

	first, err := store.ThreadFor(t.Context(), agent, o.DraftID, "usr_ana")
	if err != nil || first == "" {
		t.Fatalf("ThreadFor = %q (%v), want a thread", first, err)
	}
	again, err := store.ThreadFor(t.Context(), agent, o.DraftID, "usr_ana")
	if err != nil || again != first {
		t.Errorf("ThreadFor again = %q (%v), want %q", again, err, first)
	}
	turn := mustBegin(t, store, agent, "usr_ana", "k", o.Revision, 1)
	if turn.Thread != first {
		t.Errorf("the author's turns live in %q, the rehearsal's thread is %q", turn.Thread, first)
	}
	other, err := store.ThreadFor(t.Context(), agent, o.DraftID, "usr_bia")
	if err != nil || other == first {
		t.Errorf("another author's thread = %q (%v), want one of their own", other, err)
	}
}
