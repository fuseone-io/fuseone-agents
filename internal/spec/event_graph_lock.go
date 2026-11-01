package spec

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

/*
eventGraphLock is the advisory lock that serialises a publication's cycle
judgement with every change to which versions are current (#227).

The graph is drawn from the current version of every agent, so it changes
when a version is published — the newest is current until somebody chooses —
and when one is made current. Each of those takes this lock in its own
transaction, and a publication judges the graph only while holding it: a
judgement can never be made against a graph another writer is about to change
underneath it.

Transaction-scoped, so it is released by the commit or rollback that ends the
write and cannot outlive a connection returned to the pool. One installation
key, because a cycle can run through any agents: a lock per agent would let
the two halves of a loop each hold their own and miss each other. The value
is arbitrary and only has to be unique among this database's advisory locks.
*/
const eventGraphLock int64 = 0x0227_e7e7_9a4f_10c4

func lockEventGraph(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock($1)`, eventGraphLock); err != nil {
		return fmt.Errorf("lock the event graph: %w", err)
	}
	return nil
}

// underEventGraphLock runs a write that changes a current version in its own
// transaction, holding the event graph's lock.
func underEventGraphLock(ctx context.Context, pool *pgxpool.Pool, write func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("spec: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockEventGraph(ctx, tx); err != nil {
		return fmt.Errorf("spec: %w", err)
	}
	if err := write(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("spec: commit: %w", err)
	}
	return nil
}
