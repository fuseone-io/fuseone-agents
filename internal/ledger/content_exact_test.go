package ledger_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

/*
PutExactForTx keeps a seal's bytes (NT-018 phase 2, release 2b).

A seal's bytes are what its version's digest covers, so they are kept exactly or not at
all: never truncated, never written outside the transaction that writes the seal, and
never restored over a tombstone, because a tombstone may be a subject's erasure.
*/

func exactOwner(t *testing.T) string {
	return "seal-" + time.Now().Format("150405.000000000")
}

func TestPutExactForTx_keepsTheWholePayloadPastTheTruncationLimit(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	content := ledger.NewContent(pool).WithLimit(0)
	data := bytes.Repeat([]byte("x"), 3<<19) // 1.5 MiB, past the default 1 MiB
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	ref, err := content.PutExactForTx(ctx, tx, "authoring", exactOwner(t), 0, data)
	if err != nil {
		t.Fatalf("PutExactForTx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := content.Get(ctx, ref)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read back %d bytes (%v), want the %d stored", len(got), err, len(data))
	}
}

func TestPutExactForTx_pastAConfiguredLimit_refusesRatherThanTruncate(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	content := ledger.NewContent(pool).WithLimit(1024)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := content.PutExactForTx(ctx, tx, "authoring", exactOwner(t), 0, bytes.Repeat([]byte("x"), 2048)); !errors.Is(err, ledger.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestPutExactForTx_aRolledBackTransaction_leavesNoContent(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	content := ledger.NewContent(pool).WithLimit(0)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	ref, err := content.PutExactForTx(ctx, tx, "authoring", exactOwner(t), 0, []byte("sealed"))
	if err != nil {
		t.Fatalf("PutExactForTx: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := content.Get(ctx, ref); !errors.Is(err, ledger.ErrNoContent) {
		t.Fatalf("after rollback: err = %v, want ErrNoContent", err)
	}
}

func TestPutExactForTx_overATombstone_refusesAndLeavesItErased(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	content := ledger.NewContent(pool).WithLimit(0)
	owner := exactOwner(t)
	put := func() (string, error) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		ref, err := content.PutExactForTx(ctx, tx, "authoring", owner, 0, []byte("sealed text"))
		if err != nil {
			return "", err
		}
		return ref, tx.Commit(ctx)
	}
	ref, err := put()
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	if again, err := put(); err != nil || again != ref {
		t.Fatalf("the same write again = %q (%v), want the same reference", again, err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := content.EraseOwnersTx(ctx, tx, "authoring", []string{owner}, "subject"); err != nil {
		t.Fatalf("erase: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := put(); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("over the tombstone: err = %v, want ErrErased", err)
	}
	if _, err := content.Get(ctx, ref); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("the tombstone after the refused write: err = %v, want still erased", err)
	}
}
