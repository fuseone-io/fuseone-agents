package ledger_test

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The backfill is wired into 0092 itself, not only defined there. A database
the previous release left at 0091 holds finished runs with no finished_xid
column at all; upgrading it through the production runner must give the last
hour's of them a position and leave the older ones without one. The test never
calls the backfill function: if the migration stopped calling it, nothing
would.
*/
func TestMigrate_upgradeFrom0091_backfillsTheLastHoursFinishedRuns(t *testing.T) {
	pool := freshDatabase(t, "ledger/upgrade_0092")
	ctx := t.Context()
	if err := ledger.MigrateThrough(ctx, pool, "0091_webhook_github"); err != nil {
		t.Fatalf("migrate through 0091: %v", err)
	}
	if _, err := pool.Exec(ctx, `insert into runs
		(run_id, company_id, area_id, agent_id, version_id, phase, last_seq, started_at, updated_at, ended_at)
		values
		('run-recent', 'acme', 'cx', 'triage', 'v3', 'finished', 2, now() - interval '20 minutes', now(), now() - interval '10 minutes'),
		('run-old',    'acme', 'cx', 'triage', 'v3', 'finished', 2, now() - interval '4 hours',    now(), now() - interval '3 hours')`); err != nil {
		t.Fatalf("seed runs finished before 0092: %v", err)
	}

	if err := ledger.Migrate(ctx, pool); err != nil {
		t.Fatalf("upgrade to the current schema: %v", err)
	}

	positioned := map[string]bool{}
	rows, err := pool.Query(ctx, `select run_id, finished_xid is not null from runs`)
	if err != nil {
		t.Fatalf("read runs: %v", err)
	}
	for rows.Next() {
		var id string
		var has bool
		if err := rows.Scan(&id, &has); err != nil {
			t.Fatalf("scan: %v", err)
		}
		positioned[id] = has
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read runs: %v", err)
	}
	if !positioned["run-recent"] || positioned["run-old"] {
		t.Fatalf("finished_xid set after the upgrade = %v; want run-recent only", positioned)
	}
}

// freshDatabase opens an empty database of the test's own: an upgrade test
// needs a schema that stops partway, which the package's shared, fully
// migrated database is not. Like every package database it is kept between
// runs and emptied on entry, rather than dropped: DROP DATABASE waits on a
// forced checkpoint, which under the package's parallel load took seconds.
func freshDatabase(t *testing.T, name string) *pgxpool.Pool {
	t.Helper()
	testdb.Required(t)
	dsn, err := testdb.Ensure(t.Context(), os.Getenv("TEST_DATABASE_URL"), name)
	if err != nil {
		t.Fatalf("create the %s database: %v", name, err)
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(t.Context(), `drop schema public cascade; create schema public`); err != nil {
		t.Fatalf("empty the %s database: %v", name, err)
	}
	return pool
}
