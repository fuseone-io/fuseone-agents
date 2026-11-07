package testdb_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fuseone/agents/internal/testdb"
)

const contendedRow = `select 1 from contention_probe where id = 1 for update`

func contentionFixture(t *testing.T) *testdb.Contention {
	t.Helper()
	ctx := t.Context()
	admin, err := pgx.Connect(ctx, testdb.Required(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	if _, err := admin.Exec(ctx, `
		create table if not exists contention_probe (id int primary key);
		insert into contention_probe values (1) on conflict do nothing`); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return testdb.NewContention(t, 2)
}

// A writer that arrives late finds the row still held, however many other sessions were
// already waiting on it. Counting those would let go before this writer had read.
func TestOverlap_aWaitElsewhere_doesNotCountAsAWriter(t *testing.T) {
	contention := contentionFixture(t)
	ctx := context.WithoutCancel(t.Context())

	writes := []func() error{func() error {
		// Enough time for the session below to be queued on the row, and for the watch
		// to see it. Then the row has to be held still.
		time.Sleep(300 * time.Millisecond)
		tx, err := contention.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		var pgErr *pgconn.PgError
		_, err = tx.Exec(ctx, contendedRow+` nowait`)
		if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
			return errors.New("the hold was released before this writer arrived")
		}
		_ = tx.Rollback(ctx)
		_, err = contention.Pool.Exec(ctx, contendedRow)
		return err
	}}

	elsewhere, err := pgx.Connect(ctx, testdb.Required(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = elsewhere.Close(context.Background()) }()
	queued := make(chan error, 1)
	writes = append(writes, func() error {
		// One more writer the watch must see, so the wait is for two of its own and one
		// foreign session is never enough, even combined with the first to arrive.
		_, err := contention.Pool.Exec(ctx, contendedRow)
		return err
	})
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, err := elsewhere.Exec(ctx, contendedRow)
		queued <- err
	}()

	for _, err := range contention.Overlap(t, contendedRow, nil, writes) {
		if err != nil {
			t.Error(err)
		}
	}
	if err := <-queued; err != nil {
		t.Fatalf("the session elsewhere: %v", err)
	}
}
