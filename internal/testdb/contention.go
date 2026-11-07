package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// contentionWait bounds how long writers get to reach a lock, and how long they get to
// finish once it is let go. Well past anything a healthy database takes, and well inside
// what a CI job allows.
const contentionWait = 30 * time.Second

/*
Contention makes writers overlap, for a test of what holds them apart.

Started together they do not overlap: opening a connection takes far longer than a write,
so they finish one after another and a missing lock goes unseen. So Overlap holds a row. A
plain read passes that hold and a write does not, so every writer has read before any can
write, wherever its code makes it wait.

Three things keep the wait honest. The hold and the watch use connections of their own, so
blocked writers cannot starve what would release them: pgxpool's default ceiling is
max(4, NumCPU), so a test sharing one pool passes on a large machine and hangs on a small
one. Pool has room for every writer, or some would queue for a connection and never reach
a lock. And the watch counts only the backends of this Pool, named uniquely, so a wait
elsewhere in the database cannot complete the count early.
*/
type Contention struct {
	// Pool is what the writers under test must use.
	Pool *pgxpool.Pool

	application string
	holder      *pgx.Conn
	watcher     *pgx.Conn
}

// NewContention prepares for that many writers on the calling package's database, which
// must already be migrated.
func NewContention(t testing.TB, writers int) *Contention {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), contentionWait)
	defer cancel()
	dsn := Required(t)

	tag := make([]byte, 8)
	if _, err := rand.Read(tag); err != nil {
		t.Fatalf("name the writers: %v", err)
	}
	application := "contention-" + hex.EncodeToString(tag)

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse the DSN: %v", err)
	}
	config.MaxConns = int32(writers)
	config.ConnConfig.RuntimeParams["application_name"] = application
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect the writers: %v", err)
	}
	t.Cleanup(pool.Close)

	return &Contention{
		Pool: pool, application: application,
		holder: reservedConn(ctx, t, dsn), watcher: reservedConn(ctx, t, dsn),
	}
}

func reservedConn(ctx context.Context, t testing.TB, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("reserve a connection: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// Overlap holds what hold locks, runs every write, waits until each is blocked on a lock,
// lets go, and answers what each returned, in order. On failure it still lets go and waits
// for them, so no writer outlives the test.
func (c *Contention) Overlap(t testing.TB, hold string, args []any, writes []func() error) []error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), contentionWait)
	defer cancel()

	tx, err := c.holder.Begin(ctx)
	if err != nil {
		t.Fatalf("begin the hold: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	held, err := tx.Exec(ctx, hold, args...)
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	if held.RowsAffected() == 0 {
		t.Fatalf("the hold locked nothing, so nothing would make the writers wait: %s", hold)
	}

	results := make([]error, len(writes))
	done := make(chan struct{})
	finished := make(chan int, len(writes))
	for i, write := range writes {
		go func() {
			results[i] = write()
			finished <- i
		}()
	}
	go func() {
		for range writes {
			<-finished
		}
		close(done)
	}()

	waitErr := c.waitForWriters(ctx, len(writes))
	if err := tx.Rollback(ctx); err != nil && waitErr == nil {
		waitErr = fmt.Errorf("release the hold: %w", err)
	}
	select {
	case <-done:
	case <-time.After(contentionWait):
		t.Fatalf("the writers did not finish after the hold was released")
	}
	if waitErr != nil {
		t.Fatalf("%v", waitErr)
	}
	return results
}

func (c *Contention) waitForWriters(ctx context.Context, want int) error {
	waiting := 0
	for {
		err := c.watcher.QueryRow(ctx, `
			select count(*) from pg_stat_activity
			where application_name = $1 and wait_event_type = 'Lock'`, c.application).Scan(&waiting)
		if err != nil {
			return fmt.Errorf("%d of %d writers had reached a lock: %w", waiting, want, err)
		}
		if waiting == want {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%d of %d writers had reached a lock: %w", waiting, want, ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}
