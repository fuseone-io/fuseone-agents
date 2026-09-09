package ledger_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Where the precondition is checked, proved against the real store.

The contract suite proves the condition is evaluated. It cannot prove *where*:
a check made after reading the head but before the lock passes every sequential
case, and it also passes eight goroutines racing on a laptop — I measured, and
both that sabotage and removing the lock outright stayed green. Transactions
here take a fraction of a millisecond, so writers started in a loop do not
overlap; a test that merely starts several is a test that looks like an
accuser and is not one.

So the overlap is made rather than hoped for. A connection of our own holds the
run's advisory lock, every decider piles up behind it, and only then is it
released. From that moment they are genuinely simultaneous, and the two
placements answer differently:

  - checked under the lock, each decider re-reads the head and all but the
    first find the question already settled;
  - checked before it, every one of them passed while blocked and every one
    seals, which is the bug.
*/
func TestAppendIfHead_decidersReleasedTogether_sealExactlyOne(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		requireDatabase(t, dsn)
		t.Skip("TEST_DATABASE_URL is unset; the lock is a Postgres fact")
	}
	ctx := context.Background()

	const deciders = 8
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	// One connection per decider, plus the one that holds the lock. Fewer and
	// they would queue on the pool instead of on the lock, which is the thing
	// being measured.
	config.MaxConns = deciders + 2
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, `truncate run_steps, runs`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	store := ledger.NewPostgres(pool)
	if _, err := store.Append(ctx, step("run-1", domain.StepRunStarted)); err != nil {
		t.Fatalf("start: %v", err)
	}
	asked, err := store.Append(ctx, step("run-1", domain.StepApprovalRequested))
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	head := domain.StepRef{Seq: asked.Seq, Kind: domain.StepApprovalRequested}

	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin holder: %v", err)
	}
	defer func() { _ = holder.Rollback(ctx) }()
	if _, err := holder.Exec(ctx,
		`select pg_advisory_xact_lock(hashtextextended($1, 0))`, "run-1"); err != nil {
		t.Fatalf("hold the run's lock: %v", err)
	}

	outcomes := make(chan error, deciders)
	var done sync.WaitGroup
	done.Add(deciders)
	for i := range deciders {
		go func() {
			defer done.Done()
			decision := step("run-1", domain.StepApprovalDecided)
			// A key each. The precondition is what is under test, and one
			// shared key would let idempotency settle the race instead.
			decision.IdemKey = fmt.Sprintf("decider-%d", i)
			_, err := store.AppendIfHead(ctx, head, decision)
			outcomes <- err
		}()
	}

	waitForWaiters(t, pool, deciders)
	if err := holder.Rollback(ctx); err != nil {
		t.Fatalf("release the lock: %v", err)
	}
	done.Wait()
	close(outcomes)

	sealed, refused := 0, 0
	for err := range outcomes {
		switch {
		case err == nil:
			sealed++
		case errors.Is(err, domain.ErrHeadMoved):
			refused++
		default:
			t.Errorf("err = %v, want nil or ErrHeadMoved", err)
		}
	}
	if sealed != 1 || refused != deciders-1 {
		t.Errorf("%d sealed and %d refused, want 1 and %d", sealed, refused, deciders-1)
	}

	steps, err := store.Read(ctx, "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n := countKind(steps, domain.StepApprovalDecided); n != 1 {
		t.Errorf("%d decisions in the ledger, want one", n)
	}
}

// waitForWaiters blocks until every decider is queued on the advisory lock.
//
// Asked of Postgres rather than timed. A sleep long enough to be reliable is
// long enough to be slow, and one short enough to be quick is a test that
// passes for the wrong reason on a loaded machine.
func waitForWaiters(t *testing.T, pool *pgxpool.Pool, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		err := pool.QueryRow(context.Background(), `
			select count(*) from pg_locks
			where locktype = 'advisory' and not granted`).Scan(&waiting)
		if err != nil {
			t.Fatalf("read lock waiters: %v", err)
		}
		if waiting >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d deciders reached the lock", waiting, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
