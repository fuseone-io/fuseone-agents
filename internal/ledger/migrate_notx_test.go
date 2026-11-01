package ledger_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/ledger"
)

/*
A migration may opt out of the transaction the runner wraps around every
other one (#228). The opt-in is a first line of exactly
`-- fuseone:no-transaction`, and it exists for one kind of change: an index
built with create index concurrently, which PostgreSQL refuses inside a
transaction and which is the only way to index a large live table without
holding its writes for the length of the build. Such a file runs one
statement at a time, still under the migration lock, and is recorded only
after every statement succeeded — so it must be idempotent, and the tests
below hold the feed's index (0094) to that.
*/

const beforeFeedIndex = "0092_event_dispatch_cursor"

func feedIndexValid(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	var valid bool
	err := pool.QueryRow(context.Background(), `select i.indisvalid from pg_index i
		join pg_class c on c.oid = i.indexrelid where c.relname = 'runs_feed_idx'`).Scan(&valid)
	if err != nil {
		t.Fatalf("read runs_feed_idx: %v", err)
	}
	return valid
}

func recorded(t *testing.T, pool *pgxpool.Pool, version string) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`select count(*) from schema_migrations where version = $1`, version).Scan(&n); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return n == 1
}

func TestMigrate_aNoTransactionFile_runsEachStatementOnItsOwn_andAPlainOneInOne(t *testing.T) {
	pool := freshDatabase(t, "ledger/migrate_notx_probe")
	plain := "create table probe (label text, xid text);\n" +
		"insert into probe values ('plain', pg_current_xact_id()::text);\n" +
		"insert into probe values ('plain', pg_current_xact_id()::text);\n"
	marked := "-- fuseone:no-transaction\n" +
		"insert into probe values ('marked', pg_current_xact_id()::text);\n" +
		"insert into probe values ('marked', pg_current_xact_id()::text);\n"
	if err := ledger.MigrateBodies(t.Context(), pool, []ledger.TestMigration{
		{Version: "0001_plain", Body: plain}, {Version: "0002_marked", Body: marked},
	}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	xids := map[string]int{}
	rows, err := pool.Query(t.Context(), `select label, count(distinct xid) from probe group by label`)
	if err != nil {
		t.Fatalf("read probe: %v", err)
	}
	for rows.Next() {
		var label string
		var n int
		if err := rows.Scan(&label, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		xids[label] = n
	}
	if xids["plain"] != 1 || xids["marked"] != 2 {
		t.Fatalf("distinct transactions = %v; want the plain file in one, each marked statement in its own", xids)
	}
	var digest string
	if err := pool.QueryRow(t.Context(),
		`select digest from schema_migrations where version = '0002_marked'`).Scan(&digest); err != nil ||
		digest != ledger.MigrationDigest(marked) {
		t.Fatalf("recorded digest %q, %v; want the digest of the whole file", digest, err)
	}
}

/*
A failed statement leaves no record, so the next start runs the file again;
a record written first would mark as done a migration that never happened.
*/
func TestMigrate_aNoTransactionFileThatFails_isNotRecorded(t *testing.T) {
	pool := freshDatabase(t, "ledger/migrate_notx_fail")
	marked := "-- fuseone:no-transaction\n" +
		"create index concurrently if not exists missing_idx on no_such_table (id);\n"
	err := ledger.MigrateBodies(t.Context(), pool, []ledger.TestMigration{{Version: "0001_marked", Body: marked}})
	if err == nil {
		t.Fatal("a migration over a missing table succeeded")
	}
	if recorded(t, pool, "0001_marked") {
		t.Fatal("the failed migration was recorded; it would never run again")
	}
}

// stallBuild opens a write to runs and keeps it open: a concurrent index build
// waits for every writer that started before it, so the build stays in flight
// until the returned transaction ends.
func stallBuild(t *testing.T, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if _, err := tx.Exec(t.Context(), `insert into runs
		(run_id, company_id, area_id, agent_id, version_id, phase, last_seq, started_at, updated_at)
		values ('run-stall', 'acme', 'cx', 'triage', 'v3', 'running', 1, now(), now())`); err != nil {
		t.Fatalf("open a writer: %v", err)
	}
	return tx
}

// migrateInBackground runs the full migration set on its own pool.
func migrateInBackground(t *testing.T, dsn string) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		own, err := pgxpool.New(context.Background(), dsn)
		if err != nil {
			done <- err
			return
		}
		defer own.Close()
		done <- ledger.Migrate(context.Background(), own)
	}()
	return done
}

// awaitBuild waits until some session holds the lock a concurrent build
// takes on runs, and fails if any session holds or asks for the share lock a
// plain build would.
func awaitBuild(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var concurrent, plain int
		if err := pool.QueryRow(t.Context(), `select
			count(*) filter (where mode = 'ShareUpdateExclusiveLock' and granted),
			count(*) filter (where mode = 'ShareLock')
			from pg_locks where relation = 'runs'::regclass`).Scan(&concurrent, &plain); err != nil {
			t.Fatalf("read pg_locks: %v", err)
		}
		if plain > 0 {
			t.Fatalf("a session holds or awaits ShareLock on runs: a plain build, which blocks every write")
		}
		if concurrent > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no concurrent build of runs_feed_idx started")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func upgradeDSN(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	return pool.Config().ConnString()
}

/*
While the feed's index builds, a run can still be appended. The build is held
open by an earlier writer; a plain create index would queue a share lock
behind it, and every insert after would queue behind that.
*/
func TestMigrate_theFeedIndexBuild_doesNotBlockWritesToRuns(t *testing.T) {
	pool := freshDatabase(t, "ledger/migrate_notx_writes")
	if err := ledger.MigrateThrough(t.Context(), pool, beforeFeedIndex); err != nil {
		t.Fatalf("migrate through %s: %v", beforeFeedIndex, err)
	}
	stall := stallBuild(t, pool)
	done := migrateInBackground(t, upgradeDSN(t, pool))
	awaitBuild(t, pool)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `insert into runs
		(run_id, company_id, area_id, agent_id, version_id, phase, last_seq, started_at, updated_at)
		values ('run-during-build', 'acme', 'cx', 'triage', 'v3', 'running', 1, now(), now())`); err != nil {
		t.Fatalf("append a run while the index builds: %v", err)
	}

	if err := stall.Commit(t.Context()); err != nil {
		t.Fatalf("end the writer: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !feedIndexValid(t, pool) || !recorded(t, pool, "0094_runs_feed_idx") {
		t.Fatal("runs_feed_idx is not built and recorded")
	}
}

/*
A second process starting while the build runs waits its turn and succeeds.
Waiting inside pg_advisory_lock would hold a snapshot the concurrent build
must outlive, and PostgreSQL resolves that cycle by killing one of them.
*/
func TestMigrate_aProcessWaitingOnTheFeedIndexBuild_doesNotDeadlock(t *testing.T) {
	pool := freshDatabase(t, "ledger/migrate_notx_wait")
	if err := ledger.MigrateThrough(t.Context(), pool, beforeFeedIndex); err != nil {
		t.Fatalf("migrate through %s: %v", beforeFeedIndex, err)
	}
	stall := stallBuild(t, pool)
	first := migrateInBackground(t, upgradeDSN(t, pool))
	awaitBuild(t, pool)
	second := migrateInBackground(t, upgradeDSN(t, pool))
	// Past deadlock_timeout (1s), so a cycle would have been found and broken.
	time.Sleep(1500 * time.Millisecond)

	if err := stall.Commit(t.Context()); err != nil {
		t.Fatalf("end the writer: %v", err)
	}
	for i, done := range []<-chan error{first, second} {
		if err := <-done; err != nil {
			t.Errorf("process %d: %v", i+1, err)
		}
	}
}

// A crash after the build and before the record: the index is there, the
// record is not. The next start runs the file again and must succeed.
func TestMigrate_theFeedIndexAfterACrashBeforeItsRecord_runsAgainCleanly(t *testing.T) {
	pool := freshDatabase(t, "ledger/migrate_notx_crash")
	if err := ledger.MigrateThrough(t.Context(), pool, beforeFeedIndex); err != nil {
		t.Fatalf("migrate through %s: %v", beforeFeedIndex, err)
	}
	if _, err := pool.Exec(t.Context(), `create index runs_feed_idx on runs (finished_xid, run_id)
		where finished_xid is not null and phase = 'finished' and not simulated`); err != nil {
		t.Fatalf("leave the index behind: %v", err)
	}

	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate again: %v", err)
	}
	if !feedIndexValid(t, pool) || !recorded(t, pool, "0094_runs_feed_idx") {
		t.Fatal("runs_feed_idx is not valid and recorded after the re-run")
	}
}

// A concurrent build that died leaves an INVALID index of the same name, which
// create index if not exists would accept as done. It is dropped and rebuilt.
func TestMigrate_anInvalidFeedIndexLeftByAFailedBuild_isRebuilt(t *testing.T) {
	pool := freshDatabase(t, "ledger/migrate_notx_invalid")
	if err := ledger.MigrateThrough(t.Context(), pool, beforeFeedIndex); err != nil {
		t.Fatalf("migrate through %s: %v", beforeFeedIndex, err)
	}
	for _, id := range []string{"run-a", "run-b"} {
		if _, err := pool.Exec(t.Context(), `insert into runs
			(run_id, company_id, area_id, agent_id, version_id, phase, last_seq, started_at, updated_at)
			values ($1, 'acme', 'cx', 'triage', 'v3', 'running', 1, now(), now())`, id); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	// Two runs share a phase, so a unique concurrent build fails part-way and
	// leaves its index INVALID under the name the migration uses.
	if _, err := pool.Exec(t.Context(), `create unique index concurrently runs_feed_idx on runs (phase)`); err == nil {
		t.Fatal("the failing build succeeded")
	}
	if feedIndexValid(t, pool) {
		t.Fatal("the failed build left a valid index; the test stages nothing")
	}

	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !feedIndexValid(t, pool) {
		t.Fatal("runs_feed_idx is still INVALID: the leftover was taken as built")
	}
}
