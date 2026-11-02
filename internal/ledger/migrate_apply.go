package ledger

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// migrationLockPoll is how long a migrating process waits between two tries
// for the migration lock.
const migrationLockPoll = 250 * time.Millisecond

/*
takeMigrationLock waits for the migration lock by trying for it, not by
queueing on it.

A process queued inside pg_advisory_lock is a running statement with a
snapshot, and create index concurrently waits for every snapshot older than
its own to end before it finishes. The process building the index holds the
lock the queued one waits for: a cycle PostgreSQL breaks by killing one of
them. Between two tries the waiting process holds nothing, so the build only
ever waits for a try that ends at once.
*/
func takeMigrationLock(ctx context.Context, conn interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	for {
		var got bool
		if err := conn.QueryRow(ctx, `select pg_try_advisory_lock($1)`, migrationLock).Scan(&got); err != nil {
			return fmt.Errorf("take the migration lock: %w", err)
		}
		if got {
			return nil
		}
		timer := time.NewTimer(migrationLockPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("take the migration lock: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// applyPending runs, in order, each migration not yet recorded.
func applyPending(
	ctx context.Context, conn migrator, migrations []embeddedMigration, applied map[string]appliedMigration,
) error {
	for _, migration := range migrations {
		if _, done := applied[migration.version]; done {
			continue
		}
		apply := applyOne
		if outsideTransaction(migration.body) {
			apply = applyOutsideTransaction
		}
		if err := apply(ctx, conn, migration.version, migration.body, migration.digest); err != nil {
			return err
		}
	}
	return nil
}

// applyOne runs a migration and records it in the same transaction, so a
// crash mid-migration leaves neither half applied.
func applyOne(ctx context.Context, conn migrator, version string, body []byte, digest string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s: %w", version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, string(body)); err != nil {
		return fmt.Errorf("apply %s: %w", version, err)
	}
	if err := recordMigration(ctx, tx, version, digest); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func recordMigration(ctx context.Context, conn interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, version, digest string) error {
	if _, err := conn.Exec(ctx,
		`insert into schema_migrations (version, digest, digest_source)
		 values ($1, $2, 'applied')`, version, digest); err != nil {
		return fmt.Errorf("record %s: %w", version, err)
	}
	return nil
}

/*
noTransactionMarker opts a migration out of the transaction, as its first
line and nothing else.

It exists for one kind of change: create index concurrently, which
PostgreSQL refuses inside a transaction and which is the only way to index a
large live table without holding its writes for the whole build. Everything
else keeps the transaction, where a crash leaves neither half applied.
*/
const noTransactionMarker = "-- fuseone:no-transaction"

func outsideTransaction(body []byte) bool {
	first, _, _ := strings.Cut(string(body), "\n")
	return strings.TrimRight(first, " \t\r") == noTransactionMarker
}

/*
applyOutsideTransaction runs a marked migration one statement at a time,
still under the migration lock, and records it only after every statement
succeeded. A crash in between leaves the work done and unrecorded, so the
file runs again on the next start and must be idempotent.

Before a "create index concurrently if not exists <name>", the runner drops
an INVALID index of that name — what a concurrent build that died leaves
behind, and what "if not exists" would otherwise accept as built. The check
is the runner's own fixed query on the name the parser read, not something
the file runs: a marked file holds index statements and nothing else.
*/
func applyOutsideTransaction(ctx context.Context, conn migrator, version string, body []byte, digest string) error {
	statements, err := splitStatements(string(body))
	if err != nil {
		return fmt.Errorf("apply %s: %w", version, err)
	}
	// Every table resolves before anything runs: one that does not leaves no
	// schema to look in for a leftover, and the file would fail part-way.
	for _, statement := range statements {
		if _, err := indexSchema(ctx, conn, statement); err != nil {
			return fmt.Errorf("apply %s: %w", version, err)
		}
	}
	for _, statement := range statements {
		if err := runStatement(ctx, conn, statement); err != nil {
			return fmt.Errorf("apply %s: %w", version, err)
		}
	}
	return recordMigration(ctx, conn, version, digest)
}

func runStatement(ctx context.Context, conn migrator, statement indexStatement) error {
	if statement.create {
		if err := dropInvalidIndex(ctx, conn, statement); err != nil {
			return err
		}
	}
	_, err := conn.Exec(ctx, statement.text)
	return err
}

// indexState is where a create's index lands — its table's schema, resolved
// as PostgreSQL resolves the table (qualified, or through search_path) — and
// whether an index of that name is already there, valid or not.
type indexState struct {
	schema string
	exists bool
	valid  bool
}

// indexSchema resolves a create's table with to_regclass and reads the index
// of that name in the table's schema. A table that does not resolve is an
// error. A drop has no table and nothing to resolve.
func indexSchema(ctx context.Context, conn migrator, statement indexStatement) (indexState, error) {
	if !statement.create {
		return indexState{}, nil
	}
	rows, err := conn.Query(ctx, `select n.nspname, i.indisvalid
		from pg_class t
		join pg_namespace n on n.oid = t.relnamespace
		left join pg_class c on c.relname = $2 and c.relnamespace = t.relnamespace
		left join pg_index i on i.indexrelid = c.oid
		where t.oid = to_regclass($1)`, statement.table, statement.name)
	if err != nil {
		return indexState{}, fmt.Errorf("resolve table %s: %w", statement.table, err)
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (indexState, error) {
		var st indexState
		var valid *bool
		err := row.Scan(&st.schema, &valid)
		st.exists, st.valid = valid != nil, valid != nil && *valid
		return st, err
	})
	if err != nil {
		return indexState{}, fmt.Errorf("resolve table %s: %w", statement.table, err)
	}
	if len(found) == 0 {
		return indexState{}, fmt.Errorf("table %s does not resolve", statement.table)
	}
	return found[0], nil
}

// dropInvalidIndex drops, concurrently, the index a create names when a
// failed concurrent build left it INVALID in its table's schema. A valid one
// is kept. The drop is qualified with that schema, so it acts on exactly the
// index that was found.
func dropInvalidIndex(ctx context.Context, conn migrator, statement indexStatement) error {
	st, err := indexSchema(ctx, conn, statement)
	if err != nil || !st.exists || st.valid {
		return err
	}
	if _, err := conn.Exec(ctx,
		`drop index concurrently `+pgx.Identifier{st.schema, statement.name}.Sanitize()); err != nil {
		return fmt.Errorf("drop invalid index %s.%s: %w", st.schema, statement.name, err)
	}
	return nil
}
