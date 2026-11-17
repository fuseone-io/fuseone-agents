package drafts_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The conversation's turns (NT-018 phase 3, contract 7).

A turn is idempotent by its key, one is in progress per thread, and it is paid
for in advance: the worst case is reserved against the day's ceiling before
the model is called, and the real cost settles it. A turn whose lease ran out
before it was stored is unknown, never called again, and its reservation is
kept against the ceiling.
*/

func begin(t *testing.T, store *drafts.Store, agent domain.AgentID, by domain.UserID, key string, read domain.Revision, reserve int64) (drafts.Started, error) {
	t.Helper()
	// Room for this turn whatever the day already holds: the package's
	// database is shared by its tests, and each one's turns count today.
	return store.BeginTurn(t.Context(), drafts.Beginning{
		Agent: agent, Key: key, Message: "Triar chamados de suporte.", Read: read,
		Reserve: reserve, DailyMicros: spent(t, store) + 1_000_000, Lease: time.Minute, Caller: as(by, cx),
	})
}

func mustBegin(t *testing.T, store *drafts.Store, agent domain.AgentID, by domain.UserID, key string, read domain.Revision, reserve int64) drafts.Turn {
	t.Helper()
	s, err := begin(t, store, agent, by, key, read, reserve)
	if err != nil {
		t.Fatalf("BeginTurn(%s): %v", key, err)
	}
	return s.Turn
}

func spent(t *testing.T, store *drafts.Store) int64 {
	t.Helper()
	got, err := drafts.SpentToday(t.Context(), testdb.Pool(t))
	if err != nil {
		t.Fatalf("SpentToday: %v", err)
	}
	_ = store
	return got
}

func TestBeginTurn_reservesAndKeepsTheMessageUnderTheThread(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)

	turn := mustBegin(t, store, agent, "usr_ana", "k1", o.Revision, 300)

	if turn.State != drafts.TurnInProgress || turn.Reserved != 300 || turn.Read != o.Revision {
		t.Fatalf("turn %+v", turn)
	}
	var kind, owner string
	if err := pool.QueryRow(t.Context(), `
		select c.owner_kind, c.run_id from agent_draft_turns t join run_content c on c.ref = t.message_ref
		where t.turn_id = $1`, turn.ID).Scan(&kind, &owner); err != nil {
		t.Fatalf("read the message: %v", err)
	}
	if kind != "authoring" || owner != turn.Thread {
		t.Errorf("message under %s/%s, want the thread %s", kind, owner, turn.Thread)
	}
}

// One turn in progress per thread, whatever its key; another author's
// thread is another thread.
func TestBeginTurn_oneInProgressPerThread(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	mustBegin(t, store, agent, "usr_ana", "k1", o.Revision, 100)

	for name, key := range map[string]string{"the same key": "k1", "another key": "k2"} {
		if _, err := begin(t, store, agent, "usr_ana", key, o.Revision, 100); !errors.Is(err, drafts.ErrTurnInFlight) {
			t.Errorf("%s while one is in progress = %v, want ErrTurnInFlight", name, err)
		}
	}
	mustBegin(t, store, agent, "usr_bia", "k1", o.Revision, 100)
}

// A turn retried with its key after it was stored answers what was stored:
// the model is not called again, and nothing is reserved again.
func TestBeginTurn_aKeyAlreadySettled_replaysTheStoredTurn(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	first := mustBegin(t, store, agent, "usr_ana", "k1", o.Revision, 300)
	if _, err := store.SettleTurn(t.Context(), drafts.Settlement{Agent: agent, Caller: as("usr_ana", cx), Turn: first.ID, Cost: 120, Reply: "Proponho duas ferramentas."}); err != nil {
		t.Fatalf("SettleTurn: %v", err)
	}

	again, err := begin(t, store, agent, "usr_ana", "k1", o.Revision, 300)
	if err != nil {
		t.Fatalf("BeginTurn again: %v", err)
	}
	if !again.Replayed || again.Turn.ID != first.ID || again.Turn.State != drafts.TurnDone || again.Turn.Cost != 120 {
		t.Fatalf("again = %+v, want the stored turn replayed", again)
	}
}

/*
The crash window: a turn whose lease ran out before it was stored. Retried
with its key it becomes unknown, the model is not called again, and its
reservation is kept against the day's ceiling without being recorded as a
cost.
*/
func TestBeginTurn_aKeyWhoseLeaseRanOut_isUnknownAndKeepsItsReservation(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	first := mustBegin(t, store, agent, "usr_ana", "k1", o.Revision, 300)
	if _, err := pool.Exec(t.Context(), `update agent_draft_turns set lease_until = now() - interval '1 second' where turn_id = $1`, first.ID); err != nil {
		t.Fatalf("expire the lease: %v", err)
	}
	before := spent(t, store)

	again, err := begin(t, store, agent, "usr_ana", "k1", o.Revision, 300)
	if err != nil {
		t.Fatalf("BeginTurn again: %v", err)
	}
	if !again.Replayed || again.Turn.State != drafts.TurnUnknown || again.Turn.Cost != 0 || again.Turn.Reserved != 300 {
		t.Fatalf("again = %+v, want the turn unknown, its reservation kept", again)
	}
	if after := spent(t, store); after != before {
		t.Errorf("the day's spend went from %d to %d; the kept reservation counted already", before, after)
	}
	// The thread is free again: an unknown turn is no longer in progress.
	mustBegin(t, store, agent, "usr_ana", "k2", o.Revision, 100)
}

// The day's ceiling holds the confirmed cost, the reservations in progress
// and those kept for an unknown result.
func TestBeginTurn_overTheDaysCeiling_isRefusedBeforeAnythingIsReserved(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	base := spent(t, store)

	_, err := store.BeginTurn(t.Context(), drafts.Beginning{Agent: agent, Key: "k1", Message: "m", Read: o.Revision,
		Reserve: 600, DailyMicros: base + 500, Lease: time.Minute, Caller: as("usr_ana", cx)})
	if !errors.Is(err, drafts.ErrOverCeiling) {
		t.Fatalf("a reservation over the ceiling = %v, want ErrOverCeiling", err)
	}
	if after := spent(t, store); after != base {
		t.Errorf("a refused turn reserved %d", after-base)
	}
}

// Two turns in two threads, released together, cannot both pass a ceiling
// that holds one: reservations are judged one at a time.
func TestBeginTurn_twoThreadsAtOnce_doNotPassTheCeilingTogether(t *testing.T) {
	pool := testdb.Pool(t)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, drafts.NewStore(pool), agent)
	base := spent(t, nil)
	c := testdb.NewContention(t, 2)
	store := drafts.NewStore(c.Pool)
	run := func(by domain.UserID) func() error {
		return func() error {
			_, err := store.BeginTurn(context.Background(), drafts.Beginning{Agent: agent, Key: "k", Message: "m",
				Read: o.Revision, Reserve: 300, DailyMicros: base + 500, Lease: time.Minute, Caller: as(by, cx)})
			return err
		}
	}

	results := c.Overlap(t, `select pg_advisory_xact_lock($1)`, []any{drafts.CeilingLockKey},
		[]func() error{run("usr_ana"), run("usr_bia")})

	passed, over := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			passed++
		case errors.Is(err, drafts.ErrOverCeiling):
			over++
		default:
			t.Errorf("BeginTurn = %v", err)
		}
	}
	if passed != 1 || over != 1 {
		t.Fatalf("passed %d, over %d; want one of each", passed, over)
	}
}

// Settling replaces the reservation with the real cost, success or failure.
func TestSettleTurn_theRealCostReplacesTheReservation(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	base := spent(t, store)
	done := mustBegin(t, store, agent, "usr_ana", "k1", o.Revision, 300)
	if got := spent(t, store) - base; got != 300 {
		t.Fatalf("in progress counts %d, want the reservation", got)
	}
	settled, err := store.SettleTurn(t.Context(), drafts.Settlement{Agent: agent, Caller: as("usr_ana", cx), Turn: done.ID, Cost: 120, Reply: "Pronto."})
	if err != nil || settled.Turn.State != drafts.TurnDone {
		t.Fatalf("SettleTurn = %+v (%v)", settled, err)
	}
	failed := mustBegin(t, store, agent, "usr_ana", "k2", o.Revision, 300)
	if _, err := store.SettleTurn(t.Context(), drafts.Settlement{Agent: agent, Caller: as("usr_ana", cx), Turn: failed.ID, Cost: 40, Failed: true}); err != nil {
		t.Fatalf("SettleTurn(failed): %v", err)
	}
	if got := spent(t, store) - base; got != 160 {
		t.Errorf("settled turns count %d, want the real costs 120 + 40", got)
	}
}

func TestBeginTurn_againstAnOlderETag_isStale(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	if _, err := store.Apply(t.Context(), drafts.Application{Agent: agent, Op: draft.AddTool{Tool: "crm.lookup"},
		Origin: draft.FromControl, Expect: ifMatch(t, o.Revision), Caller: as("usr_ana", cx), Validator: validator, Now: now}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	_, err := begin(t, store, agent, "usr_ana", "k1", o.Revision, 100)
	staleAt(t, err)
}

// Erasing a thread erases what was said in it: messages, replies and the
// bodies of its proposals, which can then never be accepted.
func TestEraseThread_erasesWhatWasSaidAndLeavesNothingToAccept(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	turn := mustBegin(t, store, agent, "usr_ana", "k1", o.Revision, 100)
	settled, err := store.SettleTurn(t.Context(), drafts.Settlement{Agent: agent, Caller: as("usr_ana", cx), Turn: turn.ID,
		Cost: 10, Reply: "Proponho.", Proposed: []drafts.Proposed{{Op: draft.AddTool{Tool: "crm.lookup"}}}, Validator: validator, Now: now})
	if err != nil || len(settled.Proposals) != 1 {
		t.Fatalf("SettleTurn = %+v (%v), want one proposal", settled, err)
	}
	p := settled.Proposals[0]

	if err := store.EraseThread(t.Context(), agent, as("usr_bia", cx)); err != nil {
		t.Fatalf("EraseThread by an author with no thread: %v", err)
	}
	if err := store.EraseThread(t.Context(), agent, as("usr_ana", cx)); err != nil {
		t.Fatalf("EraseThread: %v", err)
	}

	var kept int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from run_content where owner_kind = 'authoring' and run_id = $1 and erased_at is null`,
		turn.Thread).Scan(&kept); err != nil || kept != 0 {
		t.Errorf("%d bodies of the thread still kept (%v)", kept, err)
	}
	if _, err := accept(t, store, agent, p.ID, as("usr_ana", cx)); !errors.Is(err, drafts.ErrNotFound) {
		t.Errorf("Accept after the thread was erased = %v, want ErrNotFound", err)
	}
	turns, err := store.Turns(t.Context(), agent, as("usr_ana", cx))
	if err != nil || len(turns) != 1 || turns[0].Message != "" || !turns[0].Erased {
		t.Errorf("turns after erasure = %+v (%v), want the turn kept, its words gone", turns, err)
	}
}
