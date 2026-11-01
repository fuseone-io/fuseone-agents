package ledger_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A run finished by something that knows nothing of finished_xid — a worker on
the previous version during a rolling update, a binary rolled back, or a row
inserted straight into its terminal phase — still reaches the feed. Left
NULL, the feed would exclude it for ever.
*/
func TestFinishedSince_aRunFinishedWithoutNamingTheColumn_stillAppears(t *testing.T) {
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

	mustAppend(t, s, step("run-legacy", domain.StepRunStarted))
	if _, err := pool.Exec(ctx, `update runs set phase = 'finished', ended_at = now(), updated_at = now()
		where run_id = 'run-legacy'`); err != nil {
		t.Fatalf("legacy update: %v", err)
	}
	if _, err := pool.Exec(ctx, `insert into runs
		(run_id, company_id, area_id, agent_id, version_id, phase, last_seq, started_at, updated_at)
		values ('run-inserted', 'acme', 'cx', 'triage', 'v3', 'finished', 2, now(), now())`); err != nil {
		t.Fatalf("terminal insert: %v", err)
	}

	got, err := s.FinishedSince(ctx, domain.FinishCursor{}, 10)
	if err != nil {
		t.Fatalf("FinishedSince: %v", err)
	}
	ids := map[domain.RunID]bool{}
	for _, id := range feedIDs(got) {
		ids[id] = true
	}
	if !ids["run-legacy"] || !ids["run-inserted"] {
		t.Fatalf("feed = %v; want both runs finished without naming finished_xid", feedIDs(got))
	}
}

/*
A run that finished before the feed existed is marked so by migration 0092
(position 0). Touched by a later update — a future migration, a repair — it
must not be stamped as if it had just finished, or its events would be
dispatched years late.
*/
func TestFinishedSince_aRunFinishedBeforeTheFeedExisted_staysOut(t *testing.T) {
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

	finishRunIn(t, s, "run-history")
	if _, err := pool.Exec(ctx, `update runs set finished_xid = '0' where run_id = 'run-history'`); err != nil {
		t.Fatalf("mark as history: %v", err)
	}
	if _, err := pool.Exec(ctx, `update runs set updated_at = now() where run_id = 'run-history'`); err != nil {
		t.Fatalf("later update: %v", err)
	}

	got, err := s.FinishedSince(ctx, domain.FinishCursor{}, 10)
	if err != nil || len(got) != 0 {
		t.Fatalf("feed = %v, %v; a run from before the feed is never offered", feedIDs(got), err)
	}
}

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
