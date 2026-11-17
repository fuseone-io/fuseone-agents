package drafts_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A lapsed lease against the turn's own settlement (NT-018 phase 3, contract 7).

The call outlived its lease and its retry arrives while the settlement is
being written. Whichever wins, a settled turn stays settled: a retry judges
the lease only of a turn still in progress once it holds the row.
*/
func TestBeginTurn_aRetryBehindTheSettlement_neverUndoesIt(t *testing.T) {
	pool := testdb.Pool(t)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, drafts.NewStore(pool), agent)
	first := mustBegin(t, drafts.NewStore(pool), agent, "usr_ana", "k1", o.Revision, 300)
	if _, err := pool.Exec(t.Context(), `update agent_draft_turns set lease_until = now() - interval '1 second' where turn_id = $1`, first.ID); err != nil {
		t.Fatalf("expire the lease: %v", err)
	}
	c := testdb.NewContention(t, 1)
	store := drafts.NewStore(c.Pool)
	var again drafts.Started
	retry := func() error {
		var err error
		again, err = store.BeginTurn(context.Background(), drafts.Beginning{Agent: agent, Key: "k1", Message: "m",
			Read: o.Revision, Reserve: 300, DailyMicros: spent(t, nil) + 1_000_000, Lease: time.Minute, Caller: as("usr_ana", cx)})
		return err
	}

	// The settlement, holding the turn's row, wins.
	results := c.OverlapCommitting(t, `
		update agent_draft_turns set state = 'done', cost_micros = 120, settled_at = now() where turn_id = $1`,
		[]any{first.ID}, []func() error{retry})

	if results[0] != nil || again.Turn.State != drafts.TurnDone {
		t.Errorf("the retry = %+v (%v), want the settled turn", again.Turn, results[0])
	}
	var state string
	if err := pool.QueryRow(t.Context(), `select state from agent_draft_turns where turn_id = $1`, first.ID).Scan(&state); err != nil || state != drafts.TurnDone {
		t.Errorf("the turn is %q (%v) after the retry, want it still done", state, err)
	}
}

// A turn whose lease ran out holds the thread no longer, whichever key comes
// next: the client that lost the key is not locked out of its own thread.
func TestBeginTurn_aLapsedTurn_doesNotHoldTheThreadAgainstANewKey(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	lapsed := mustBegin(t, store, agent, "usr_ana", "k1", o.Revision, 300)
	if _, err := pool.Exec(t.Context(), `update agent_draft_turns set lease_until = now() - interval '1 second' where turn_id = $1`, lapsed.ID); err != nil {
		t.Fatalf("expire the lease: %v", err)
	}

	mustBegin(t, store, agent, "usr_ana", "k2", o.Revision, 100)

	var state string
	if err := pool.QueryRow(t.Context(), `select state from agent_draft_turns where turn_id = $1`, lapsed.ID).Scan(&state); err != nil || state != drafts.TurnUnknown {
		t.Errorf("the lapsed turn is %q (%v), want unknown, its reservation kept", state, err)
	}
}
