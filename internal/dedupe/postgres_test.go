package dedupe_test

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/dedupe"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
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
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; cross-run dedupe is a Postgres coordination fact")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `truncate tool_effect_dedupe, run_steps, runs`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return dedupe.NewPostgres(pool), pool
}
