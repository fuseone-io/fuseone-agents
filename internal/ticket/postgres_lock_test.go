package ticket_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/ticket"
)

func TestPostgres_claimersReleasedTogether_claimExactlyOnce(t *testing.T) {
	pool, store := ticketPostgres(t)
	opened := mustOpen(t, store, "event-lock")
	_, changed, err := store.AwaitApproval(t.Context(), ticket.ApprovalInput{
		Ref: opened.Current.Ref, RunID: "run-lock", AtSeq: 9,
		Snapshot: content("snapshot-lock"), At: now.Add(time.Minute),
	})
	if err != nil || !changed {
		t.Fatalf("AwaitApproval = (%v, %v)", changed, err)
	}
	holder, held := holdTicket(t, pool, opened.Key)
	input := ticket.ClaimInput{
		Ref: opened.Current.Ref, RunID: "run-lock", ApprovalAtSeq: 9,
		At: now.Add(2 * time.Minute),
	}
	results := startClaims(t.Context(), store, input, 8)
	waitForTicketWaiters(t, pool, held, 8)
	if err := holder.Rollback(t.Context()); err != nil {
		t.Fatalf("release ticket lock: %v", err)
	}
	assertOneClaim(t, results, 8)
}

type claimResult struct {
	claimed bool
	err     error
}

func startClaims(
	ctx context.Context, store ticket.Store, input ticket.ClaimInput, count int,
) <-chan claimResult {
	results := make(chan claimResult, count)
	var wg sync.WaitGroup
	wg.Add(count)
	for range count {
		go func() {
			defer wg.Done()
			_, claimed, err := store.ClaimExecution(ctx, input)
			results <- claimResult{claimed: claimed, err: err}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}

func assertOneClaim(t *testing.T, results <-chan claimResult, want int) {
	t.Helper()
	claimed, total := 0, 0
	for result := range results {
		total++
		if result.err != nil {
			t.Errorf("ClaimExecution: %v", result.err)
		}
		if result.claimed {
			claimed++
		}
	}
	if total != want || claimed != 1 {
		t.Fatalf("%d results with %d claims, want %d and 1", total, claimed, want)
	}
}

func ticketPostgres(t *testing.T) (*pgxpool.Pool, ticket.Store) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; the lock is a Postgres fact")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	config.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		`truncate governed_ticket_events, governed_ticket_revisions, governed_tickets`); err != nil {
		t.Fatalf("clean tickets: %v", err)
	}
	return pool, ticket.NewPostgres(pool)
}

type heldLock struct {
	database uint32
	classID  uint32
	objID    uint32
	objSubID int16
}

func holdTicket(
	t *testing.T, pool *pgxpool.Pool, key domain.TicketKey,
) (pgx.Tx, heldLock) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin holder: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	var pid int
	if err := tx.QueryRow(t.Context(), `select pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("identify holder: %v", err)
	}
	if _, err := tx.Exec(t.Context(),
		`select pg_advisory_xact_lock(hashtextextended($1, 0))`, "ticket:"+string(key)); err != nil {
		t.Fatalf("hold ticket lock: %v", err)
	}
	var held heldLock
	err = pool.QueryRow(t.Context(), `
		select database, classid, objid, objsubid from pg_locks
		where locktype = 'advisory' and granted and pid = $1`, pid).
		Scan(&held.database, &held.classID, &held.objID, &held.objSubID)
	if err != nil {
		t.Fatalf("read held lock: %v", err)
	}
	return tx, held
}

func waitForTicketWaiters(t *testing.T, pool *pgxpool.Pool, held heldLock, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		err := pool.QueryRow(t.Context(), `
			select count(*) from pg_locks
			where locktype = 'advisory' and not granted
			  and database = $1 and classid = $2 and objid = $3 and objsubid = $4`,
			held.database, held.classID, held.objID, held.objSubID).Scan(&waiting)
		if err != nil {
			t.Fatalf("read waiters: %v", err)
		}
		if waiting == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d claimers reached the ticket lock", waiting, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
