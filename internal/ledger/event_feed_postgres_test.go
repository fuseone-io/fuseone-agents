package ledger_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A run that commits after an older transaction is still in flight is held
back until that transaction ends (#203). Without it: the older transaction
finishes a run of its own and commits after a sweep read the newer run and
moved the cursor past both positions — and the older run lands behind the
cursor, never dispatched.
*/
func TestFinishedSince_aRunCommittedPastAnOpenTransaction_waitsForIt(t *testing.T) {
	dsn := testdb.DSN(t)
	if dsn == "" {
		requireDatabase(t, dsn)
		t.Skip("TEST_DATABASE_URL is unset")
	}
	s := openPostgres(t, dsn)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	inFlight, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = inFlight.Rollback(ctx) }()
	// An older transaction holding an id, as a finish that has not committed yet.
	if _, err := inFlight.Exec(ctx, `select pg_current_xact_id()`); err != nil {
		t.Fatalf("take an id: %v", err)
	}
	finishRunIn(t, s, "run-later")

	held, err := s.FinishedSince(ctx, domain.FinishCursor{}, 10)
	if err != nil {
		t.Fatalf("FinishedSince: %v", err)
	}
	if len(held) != 0 {
		t.Fatalf("offered %v while an older transaction is open; want it held back", feedIDs(held))
	}

	if err := inFlight.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	released, err := s.FinishedSince(ctx, domain.FinishCursor{}, 10)
	if err != nil {
		t.Fatalf("FinishedSince: %v", err)
	}
	if got := feedIDs(released); len(got) != 1 || got[0] != "run-later" {
		t.Fatalf("after the older transaction ended: %v, want [run-later]", got)
	}
}
