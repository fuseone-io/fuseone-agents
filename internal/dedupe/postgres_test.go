package dedupe_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/dedupe"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

func TestReserve_confirmsOnlyAfterASuccessfulToolReturn(t *testing.T) {
	store, _ := newStore(t)
	key := testKey()
	now := time.Date(2026, 8, 25, 17, 30, 0, 0, time.UTC)

	first, err := store.Reserve(t.Context(), key, "run-a", 10*time.Second, now)
	if err != nil {
		t.Fatalf("Reserve first: %v", err)
	}
	if first.State != dedupe.StateReserved || first.RunID != "run-a" {
		t.Fatalf("first reserve = %+v, want this run to own the reservation", first)
	}
	second, err := store.Reserve(t.Context(), key, "run-b", 10*time.Second, now.Add(time.Second))
	if err != nil {
		t.Fatalf("Reserve second: %v", err)
	}
	if second.State != dedupe.StatePending || second.RunID != "run-a" {
		t.Fatalf("second reserve = %+v, want pending held by run-a", second)
	}

	if err := store.Confirm(t.Context(), key, "run-a", 7, time.Hour, now.Add(2*time.Second)); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	confirmed, ok, err := store.Lookup(t.Context(), key, now.Add(3*time.Second))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !ok || confirmed.State != dedupe.StateConfirmed ||
		confirmed.RunID != "run-a" || confirmed.Seq != 7 {
		t.Fatalf("Lookup = %+v/%v, want confirmed run-a #7", confirmed, ok)
	}
	third, err := store.Reserve(t.Context(), key, "run-c", 10*time.Second, now.Add(4*time.Second))
	if err != nil {
		t.Fatalf("Reserve third: %v", err)
	}
	if third.State != dedupe.StateConfirmed || third.RunID != "run-a" || third.Seq != 7 {
		t.Fatalf("third reserve = %+v, want duplicate of confirmed run-a #7", third)
	}
}

func TestReserve_allowsOnlyOneConcurrentOwner(t *testing.T) {
	store, _ := newStore(t)
	key := testKey()
	now := time.Date(2026, 8, 25, 18, 20, 0, 0, time.UTC)
	const attempts = 12

	type outcome struct {
		run domain.RunID
		rec dedupe.Record
		err error
	}
	start := make(chan struct{})
	results := make(chan outcome, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		runID := domain.RunID(fmt.Sprintf("run-%02d", i))
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec, err := store.Reserve(t.Context(), key, runID, time.Minute, now)
			results <- outcome{run: runID, rec: rec, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var (
		owner   domain.RunID
		winners int
		pending []dedupe.Record
	)
	for got := range results {
		if got.err != nil {
			t.Fatalf("%s Reserve: %v", got.run, got.err)
		}
		switch got.rec.State {
		case dedupe.StateReserved:
			winners++
			owner = got.rec.RunID
		case dedupe.StatePending:
			pending = append(pending, got.rec)
		default:
			t.Fatalf("%s Reserve = %+v, want reserved or pending", got.run, got.rec)
		}
	}
	if winners != 1 || len(pending) != attempts-1 {
		t.Fatalf("winners=%d pending=%d, want 1/%d", winners, len(pending), attempts-1)
	}
	for _, rec := range pending {
		if rec.RunID != owner {
			t.Fatalf("pending reservation points to %s, want owner %s", rec.RunID, owner)
		}
	}
}

func TestRelease_afterFailedCallLetsTheNextRunReserve(t *testing.T) {
	store, _ := newStore(t)
	key := testKey()
	now := time.Date(2026, 8, 25, 17, 40, 0, 0, time.UTC)

	if _, err := store.Reserve(t.Context(), key, "run-a", time.Minute, now); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := store.Release(t.Context(), key, "run-a"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	next, err := store.Reserve(t.Context(), key, "run-b", time.Minute, now.Add(time.Second))
	if err != nil {
		t.Fatalf("Reserve next: %v", err)
	}
	if next.State != dedupe.StateReserved || next.RunID != "run-b" {
		t.Fatalf("next = %+v, want run-b to reserve after failure released run-a", next)
	}
}

func TestPendingReservationExpiresSilently(t *testing.T) {
	store, _ := newStore(t)
	key := testKey()
	now := time.Date(2026, 8, 25, 17, 50, 0, 0, time.UTC)

	if _, err := store.Reserve(t.Context(), key, "run-a", 5*time.Second, now); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	next, err := store.Reserve(t.Context(), key, "run-b", 5*time.Second, now.Add(6*time.Second))
	if err != nil {
		t.Fatalf("Reserve after expiry: %v", err)
	}
	if next.State != dedupe.StateReserved || next.RunID != "run-b" {
		t.Fatalf("next = %+v, want expired pending reservation replaced", next)
	}
}

func TestConfirmWrongRunDoesNotMarkDone(t *testing.T) {
	store, _ := newStore(t)
	key := testKey()
	now := time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC)

	if _, err := store.Reserve(t.Context(), key, "run-a", time.Minute, now); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := store.Confirm(t.Context(), key, "run-b", 3, time.Hour, now.Add(time.Second)); !errors.Is(err, dedupe.ErrReservationNotHeld) {
		t.Fatalf("Confirm wrong run = %v, want ErrReservationNotHeld", err)
	}
	got, ok, err := store.Lookup(t.Context(), key, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !ok || got.State != dedupe.StatePending || got.RunID != "run-a" {
		t.Fatalf("Lookup = %+v/%v, want run-a still pending", got, ok)
	}
}

func TestZeroTimeIsRejected(t *testing.T) {
	store, _ := newStore(t)
	if _, _, err := store.Lookup(t.Context(), testKey(), time.Time{}); err == nil {
		t.Fatal("Lookup accepted a zero observation time")
	}
	if _, err := store.Reserve(t.Context(), testKey(), "run-a", time.Minute, time.Time{}); err == nil {
		t.Fatal("Reserve accepted a zero observation time")
	}
	if err := store.Confirm(t.Context(), testKey(), "run-a", 1, time.Hour, time.Time{}); err == nil {
		t.Fatal("Confirm accepted a zero observation time")
	}
}

func TestScopeIsPartOfTheDedupeKey(t *testing.T) {
	store, _ := newStore(t)
	now := time.Date(2026, 8, 25, 18, 10, 0, 0, time.UTC)
	acme := testKey()
	otherArea := acme
	otherArea.Scope.Area = "support"
	otherCompany := acme
	otherCompany.Scope.Company = "globex"

	if _, err := store.Reserve(t.Context(), acme, "run-a", time.Minute, now); err != nil {
		t.Fatalf("Reserve acme: %v", err)
	}
	for _, tc := range []struct {
		name string
		key  dedupe.Key
		run  domain.RunID
	}{
		{name: "other area", key: otherArea, run: "run-b"},
		{name: "other company", key: otherCompany, run: "run-c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := store.Reserve(t.Context(), tc.key, tc.run, time.Minute, now.Add(time.Second))
			if err != nil {
				t.Fatalf("Reserve: %v", err)
			}
			if got.State != dedupe.StateReserved || got.RunID != tc.run {
				t.Fatalf("Reserve = %+v, want %s to reserve independently", got, tc.run)
			}
		})
	}
}

func testKey() dedupe.Key {
	return dedupe.Key{
		Scope:       domain.Scope{Company: "acme", Area: "ops"},
		AgentID:     "triage",
		Tool:        "github.create_issue",
		Fingerprint: "sha256:1234567890abcdef",
	}
}

func newStore(t *testing.T) (*dedupe.Postgres, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	if _, err := pool.Exec(t.Context(), `truncate tool_effect_dedupe, run_steps, runs`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return dedupe.NewPostgres(pool), pool
}

// The loser of a reservation race must get the winner's record, never an
// error. The race is staged rather than hoped for: the winner's row is held
// uncommitted, the loser's Reserve is made to wait on it in ON CONFLICT, and
// only then does the winner commit. The loser's upsert is then excluded by
// its own predicate, and a fallback read on the statement's snapshot — taken
// before the commit — cannot see the row either (#145).
func TestReserve_aLoserThatWaitedOnTheWinnersCommit_getsTheWinnersRecord(t *testing.T) {
	store, pool := newStore(t)
	key := testKey()
	now := time.Date(2026, 8, 25, 18, 20, 0, 0, time.UTC)

	winner, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin the winner: %v", err)
	}
	defer func() { _ = winner.Rollback(context.Background()) }()
	if _, err := winner.Exec(t.Context(), `
		insert into tool_effect_dedupe (
			company_id, area_id, agent_id, tool_id, fingerprint,
			status, run_id, seq, reserved_at, confirmed_at, expires_at, updated_at
		) values ($1, $2, $3, $4, $5, 'pending', 'run-winner', 0, $6, null, $7, $6)`,
		key.Scope.Company, key.Scope.Area, key.AgentID, key.Tool, key.Fingerprint,
		now, now.Add(time.Minute)); err != nil {
		t.Fatalf("hold the winner's row: %v", err)
	}

	type outcome struct {
		rec dedupe.Record
		err error
	}
	lost := make(chan outcome, 1)
	go func() {
		rec, err := store.Reserve(t.Context(), key, "run-loser", time.Minute, now)
		lost <- outcome{rec: rec, err: err}
	}()

	waitForLockWait(t, pool)
	if err := winner.Commit(t.Context()); err != nil {
		t.Fatalf("commit the winner: %v", err)
	}

	got := <-lost
	if got.err != nil {
		t.Fatalf("the loser's Reserve = %v, want the winner's record", got.err)
	}
	if got.rec.State != dedupe.StatePending || got.rec.RunID != "run-winner" {
		t.Fatalf("the loser got %+v, want the winner's pending reservation", got.rec)
	}
}

// waitForLockWait returns once some backend is blocked on a lock touching the
// dedupe table — the loser parked behind the winner's uncommitted row.
func waitForLockWait(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := pool.QueryRow(t.Context(), `
			select count(*) from pg_stat_activity
			where wait_event_type = 'Lock' and query like '%tool_effect_dedupe%'
			  and pid <> pg_backend_pid()`).Scan(&waiting); err != nil {
			t.Fatalf("read lock waits: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the loser never waited on the winner's row; the race was not staged")
}
