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

A statement that is a select generates statements: each row it returns is
run in turn. That is how a file says "drop this index concurrently if a
failed build left it invalid", which plain DDL cannot make conditional.
*/
func applyOutsideTransaction(ctx context.Context, conn migrator, version string, body []byte, digest string) error {
	statements, err := splitStatements(string(body))
	if err != nil {
		return fmt.Errorf("apply %s: %w", version, err)
	}
	for _, statement := range statements {
		if err := runStatement(ctx, conn, statement); err != nil {
			return fmt.Errorf("apply %s: %w", version, err)
		}
	}
	return recordMigration(ctx, conn, version, digest)
}

func runStatement(ctx context.Context, conn migrator, statement string) error {
	if !isGenerator(statement) {
		_, err := conn.Exec(ctx, statement)
		return err
	}
	rows, err := conn.Query(ctx, statement)
	if err != nil {
		return err
	}
	generated, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, one := range generated {
		if _, err := conn.Exec(ctx, one); err != nil {
			return err
		}
	}
	return nil
}

/*
splitStatements cuts a marked file into statements: each ends at a line
ending in a semicolon, comment lines never end one, and a chunk of only
comments is no statement. Dollar quoting is refused rather than parsed — a
function body has no business in a file that exists to build an index.
*/
func splitStatements(body string) ([]string, error) {
	if strings.Contains(body, "$$") {
		return nil, fmt.Errorf("a no-transaction migration may not use dollar quoting")
	}
	var out []string
	var current []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		current = append(current, line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") || !strings.HasSuffix(trimmed, ";") {
			continue
		}
		out = append(out, strings.Join(current, "\n"))
		current = nil
	}
	if code(strings.Join(current, "\n")) != "" {
		return nil, fmt.Errorf("a no-transaction migration ends in an unterminated statement")
	}
	return out, nil
}

// isGenerator reports whether a statement's code begins with select.
func isGenerator(statement string) bool {
	return strings.HasPrefix(strings.ToLower(code(statement)), "select")
}

// code is a statement without its comment lines.
func code(statement string) string {
	var kept []string
	for _, line := range strings.Split(statement, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "--") {
			kept = append(kept, trimmed)
		}
	}
	return strings.Join(kept, " ")
}
