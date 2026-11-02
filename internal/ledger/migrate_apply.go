package ledger

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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
is the runner's own fixed query, not something the file runs: a marked file
holds index statements and nothing else.
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
	if name, ok := createIfNotExistsName(statement); ok {
		if err := dropInvalidIndex(ctx, conn, name); err != nil {
			return err
		}
	}
	_, err := conn.Exec(ctx, statement)
	return err
}

// dropInvalidIndex drops, concurrently, an index of that name in the current
// schema that a failed concurrent build left INVALID. A valid one is kept.
func dropInvalidIndex(ctx context.Context, conn migrator, name string) error {
	rows, err := conn.Query(ctx, `select i.indisvalid from pg_index i
		join pg_class c on c.oid = i.indexrelid
		where c.relname = $1 and c.relnamespace = current_schema()::regnamespace`, name)
	if err != nil {
		return fmt.Errorf("check index %s: %w", name, err)
	}
	valid, err := pgx.CollectRows(rows, pgx.RowTo[bool])
	if err != nil {
		return fmt.Errorf("check index %s: %w", name, err)
	}
	if len(valid) == 0 || valid[0] {
		return nil
	}
	if _, err := conn.Exec(ctx, `drop index concurrently `+pgx.Identifier{name}.Sanitize()); err != nil {
		return fmt.Errorf("drop invalid index %s: %w", name, err)
	}
	return nil
}

// createIfNotExists captures the index a create-if-not-exists names.
var createIfNotExists = regexp.MustCompile(`^create index concurrently if not exists ([a-z_][a-z0-9_]*) on `)

func createIfNotExistsName(statement string) (string, bool) {
	m := createIfNotExists.FindStringSubmatch(normalised(statement))
	if m == nil {
		return "", false
	}
	return m[1], true
}

/*
splitStatements cuts a marked file into statements: each ends at a line
ending in a semicolon, comment lines never end one, and a chunk of only
comments is no statement. Every statement is checked against the allowed
list before any runs, so a file outside it fails having touched nothing.
*/
func splitStatements(body string) ([]string, error) {
	statements, err := cutStatements(body)
	if err != nil {
		return nil, err
	}
	for _, statement := range statements {
		if err := allowedOutsideTransaction(statement); err != nil {
			return nil, err
		}
	}
	return statements, nil
}

/*
The only statements a marked file may hold: build or drop an index
concurrently. That is the whole reason the opt-in exists. Nothing else runs —
not a select either, since a select runs functions and functions have effects
(setval, advisory locks) that no later refusal can take back.
*/
var allowedNoTransaction = regexp.MustCompile(
	`^(create index concurrently (if not exists )?|drop index concurrently (if exists )?)\S`)

// dollarQuote matches any dollar quote, $$ or named ($body$): quoting is
// refused rather than parsed, since a quoted body could hide any statement.
var dollarQuote = regexp.MustCompile(`\$[A-Za-z_]*\$`)

var errDollarQuote = errors.New("a no-transaction migration may not use dollar quoting")

func allowedOutsideTransaction(statement string) error {
	if dollarQuote.MatchString(statement) {
		return errDollarQuote
	}
	if normal := normalised(statement); !allowedNoTransaction.MatchString(normal) {
		return fmt.Errorf("a no-transaction migration may only create or drop an index concurrently, not %q", normal)
	}
	return nil
}

// normalised is a statement's code, lower case, with runs of space as one.
func normalised(statement string) string {
	return strings.ToLower(strings.Join(strings.Fields(code(statement)), " "))
}

func cutStatements(body string) ([]string, error) {
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
