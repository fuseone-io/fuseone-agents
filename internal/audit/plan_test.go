package audit_test

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/audit"
	"github.com/fuseone/agents/internal/ledger"
)

/*
The trail has to stay readable when the ledger is large.

The console asks for a page of a hundred entries; behind it sits every step of
every run this installation has ever made. If that page is answered by walking
the ledger, it is a screen that works in a demo and times out in a customer —
and it fails a year in, when the trail is the one thing somebody needs.

The assertion is about scans rather than milliseconds. A timing test on a
laptop measures the laptop; the sequential-scan counter measures the plan.
*/
func TestRead_ledgerHoldsManySteps_answersWithoutScanningIt(t *testing.T) {
	pool := volumePool(t, 40_000)
	reader := audit.NewPostgres(pool)

	// The instrument first. A zero below has to be a fact about the read and
	// not about statistics that had not been flushed yet.
	before := seqScans(t, pool)
	if _, err := pool.Exec(t.Context(),
		`select count(*) from run_steps where payload->>'nothing' = 'here'`); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if seqScans(t, pool) == before {
		t.Fatal("the counter did not move for a query that must scan; this test cannot tell")
	}

	before = seqScans(t, pool)
	if _, _, err := reader.Read(t.Context(), audit.Filter{}, 100); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := seqScans(t, pool) - before; got != 0 {
		t.Errorf("one page of the trail scanned run_steps %d times; it must be answered from an index", got)
	}
}

// volumePool opens a database holding many steps, on a single connection.
//
// One connection because the statistics this test reads are per-backend until
// they are flushed, and a pool that answered the read on one connection and
// the count on another would report whatever happened to have arrived.
func volumePool(t *testing.T, steps int) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the volume suite")
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	cfg.MaxConns, cfg.MinConns = 1, 1

	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `truncate run_steps, runs, admin_events`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	seedSteps(t, pool, steps)
	return pool
}

// seedSteps writes the ledger straight, not through the store.
//
// Forty thousand steps appended one at a time would make this a benchmark of
// the writer. What it needs is a table the planner will not read whole out of
// sheer smallness.
func seedSteps(t *testing.T, pool *pgxpool.Pool, steps int) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `
		insert into run_steps (
			run_id, seq, kind, company_id, area_id, agent_id, version_id,
			payload, at, prev_hash, hash)
		select 'run_' || (n / 20)::text, (n % 20) + 1,
		       (array['planned','gate_decided','tool_called','tool_returned','approval_decided'])[1 + n % 5],
		       'acme', (array['finance','support','ops'])[1 + n % 3],
		       'agent_' || (n % 40)::text, 'v1',
		       jsonb_build_object('verdict', 1 + n % 4, 'approved', n % 2 = 0,
		                          'tool', 'erp.invoice.read'),
		       now() - (n || ' seconds')::interval,
		       case when n % 20 = 0 then null else sha256(n::text::bytea) end,
		       sha256(n::text::bytea)
		from generate_series(0, $1::int - 1) n`, steps)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `analyze run_steps`); err != nil {
		t.Fatalf("analyze: %v", err)
	}
}

// seqScans counts sequential scans of the ledger, partitions included.
func seqScans(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `select pg_stat_force_next_flush()`); err != nil {
		t.Fatalf("flush statistics: %v", err)
	}
	var n int64
	if err := pool.QueryRow(t.Context(), `
		select coalesce(sum(seq_scan), 0) from pg_stat_user_tables
		where relname = 'run_steps' or relname like 'run_steps_p%'`).Scan(&n); err != nil {
		t.Fatalf("read statistics: %v", err)
	}
	return n
}
