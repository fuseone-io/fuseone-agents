package ledger_test

import (
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
The ledger is partitioned, and the partitioning must not cost an invariant.

Splitting a table by time is the ordinary way to keep a growing one
maintainable, and the ordinary way to do it here would have been to partition
on the step's own timestamp. That quietly breaks two things this platform
depends on. Postgres requires the partition key inside every unique constraint,
so the primary key that enforces one writer per run (NF-15) and the unique
index that enforces idempotency (Gate check 6) would both stop being global —
and a run still open when the month turns would have its steps split across two
partitions, where neither constraint can see the other half.

So the key is the run's opening time, which every step of a run shares. The
tests below are the ones that fail if anybody ever changes it back.
*/
func TestAppend_runSpansAMonthBoundary_keepsItsStepsInOnePartition(t *testing.T) {
	store, pool := partitioned(t)

	// Opened in one month, still running in the next: the ordinary case for a
	// run parked waiting for a person over a weekend, or a long compensation.
	opened := time.Date(2026, 1, 31, 23, 50, 0, 0, time.UTC)
	appendAt(t, store, "run-crosses", 1, domain.StepRunStarted, opened)
	appendAt(t, store, "run-crosses", 2, domain.StepPlanned, opened.Add(30*time.Minute))
	appendAt(t, store, "run-crosses", 3, domain.StepGateDecided, opened.Add(72*time.Hour))

	var partitions int
	if err := pool.QueryRow(t.Context(), `
		select count(distinct tableoid) from run_steps where run_id = 'run-crosses'`,
	).Scan(&partitions); err != nil {
		t.Fatalf("count partitions: %v", err)
	}
	if partitions != 1 {
		t.Errorf("the run's steps are spread over %d partitions; every constraint on"+
			" (run_id, seq) can only see one of them", partitions)
	}
}

// The primary key, not the writer, is the guarantee. Append computes the next
// sequence itself, so this goes round it and writes the collision the database
// has to refuse — which is what a partition boundary in the wrong place would
// stop it from seeing.
func TestAppend_seqRepeatsAcrossTheBoundary_stillRejectedByTheDatabase(t *testing.T) {
	store, pool := partitioned(t)

	opened := time.Date(2026, 1, 31, 23, 50, 0, 0, time.UTC)
	appendAt(t, store, "run-rewinds", 1, domain.StepRunStarted, opened)

	_, err := pool.Exec(t.Context(), `
		insert into run_steps (run_id, seq, kind, company_id, area_id,
			agent_id, version_id, at, hash)
		values ('run-rewinds', 1, 'planned', 'acme', 'cx', 'triage', 'v3', $1, $2)`,
		opened.Add(72*time.Hour), []byte("second-writer"))
	if err == nil {
		t.Fatal("a second writer claimed sequence 1 across the month boundary")
	}
}

func TestAppend_sameIdemKeyAcrossTheBoundary_stillRejected(t *testing.T) {
	store, _ := partitioned(t)

	opened := time.Date(2026, 1, 31, 23, 50, 0, 0, time.UTC)
	appendAt(t, store, "run-idem", 1, domain.StepRunStarted, opened)

	first := step("run-idem", domain.StepToolCalled)
	first.Seq, first.At, first.IdemKey = 2, opened.Add(time.Minute), "the-same-call"
	if _, err := store.Append(t.Context(), first); err != nil {
		t.Fatalf("first call: %v", err)
	}

	again := step("run-idem", domain.StepToolCalled)
	again.Seq, again.At, again.IdemKey = 3, opened.Add(72*time.Hour), "the-same-call"
	if _, err := store.Append(t.Context(), again); err == nil {
		t.Fatal("the same effect was billed twice across the month boundary")
	}
}

// A month nobody created a partition for still has to be recordable. A ledger
// that refuses an append because of its own housekeeping is worse than a large
// table.
func TestAppend_monthWithNoPartition_isStillRecorded(t *testing.T) {
	store, pool := partitioned(t)

	far := time.Date(2031, 7, 4, 9, 0, 0, 0, time.UTC)
	appendAt(t, store, "run-far", 1, domain.StepRunStarted, far)

	var caught int
	if err := pool.QueryRow(t.Context(),
		`select count(*) from run_steps_default`).Scan(&caught); err != nil {
		t.Fatalf("count the default partition: %v", err)
	}
	if caught == 0 {
		t.Error("a step outside every declared month was not caught by the default partition")
	}
}

func partitioned(t *testing.T) (Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; partitioning is a Postgres fact")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `truncate run_steps, runs`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return ledger.NewPostgres(pool), pool
}

func appendAt(t *testing.T, store Store, run string, seq int64, kind domain.StepKind, at time.Time) {
	t.Helper()
	s := step(domain.RunID(run), kind)
	s.Seq, s.At = seq, at
	if _, err := store.Append(t.Context(), s); err != nil {
		t.Fatalf("append %s seq %d: %v", kind, seq, err)
	}
}

// The job that keeps months ahead of the clock. Running it twice must be a
// no-op, because it runs on every start and every day after.
func TestEnsure_runTwice_makesTheSameMonthsOnce(t *testing.T) {
	_, pool := partitioned(t)
	at := func() time.Time { return time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC) }
	months := ledger.NewPartitions(pool, at)

	if _, err := months.Ensure(t.Context()); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	before := countPartitions(t, pool)

	if _, err := months.Ensure(t.Context()); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if after := countPartitions(t, pool); after != before {
		t.Errorf("a second pass created %d more partitions", after-before)
	}
}

func countPartitions(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from pg_inherits where inhparent = 'run_steps'::regclass`,
	).Scan(&n); err != nil {
		t.Fatalf("count partitions: %v", err)
	}
	return n
}
