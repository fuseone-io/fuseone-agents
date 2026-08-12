package ledger

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed all:sql
var migrationFS embed.FS

// Migrate applies every migration that has not run yet, in filename order.
//
// A file already recorded in schema_migrations is never re-run and never
// re-read, so an applied migration is frozen history: editing one changes
// nothing on an existing installation and diverges it from a fresh one. Fix a
// mistake with a new file.
// migrationLock is the advisory lock every migrating process queues on.
//
// Each of them migrates on the way up, and a deployment starts them together,
// so two racing on a schema neither has yet is the ordinary case. Without the
// lock both read the same empty set of applied versions, both try to create
// the same table, and the loser dies on a duplicate object — which in a
// deployment is one pod crash-looping until the other happens to finish.
//
// `create table if not exists` does not save it: two of those running at once
// still collide in the catalogue.
const migrationLock int64 = 8010081

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	files, err := fs.Glob(migrationFS, "sql/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	slices.Sort(files)

	// One connection for the whole run: a session-level lock belongs to the
	// connection that took it, and a pool would hand the unlock to another.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire for migration: %w", err)
	}
	defer conn.Release()

	// Waits rather than fails. The second process wanted the schema migrated,
	// and by the time it wakes it is.
	if _, err := conn.Exec(ctx, `select pg_advisory_lock($1)`, migrationLock); err != nil {
		return fmt.Errorf("take the migration lock: %w", err)
	}
	defer func() {
		// Released on a fresh context: a cancelled migration must still hand
		// the lock back, or every other process waits out the connection.
		unlock, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlock, `select pg_advisory_unlock($1)`, migrationLock)
	}()

	// Bootstrap: the table that records progress cannot record its own
	// creation, so it is created idempotently before anything is checked.
	if _, err := conn.Exec(ctx, `
		create table if not exists schema_migrations (
			version    text        not null primary key,
			applied_at timestamptz not null default now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	for _, file := range files {
		version := strings.TrimSuffix(strings.TrimPrefix(file, "sql/"), ".sql")
		if _, done := applied[version]; done {
			continue
		}

		body, err := migrationFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}
		if err := applyOne(ctx, conn, version, string(body)); err != nil {
			return err
		}
	}
	return nil
}

// migrator is what both a pool and one acquired connection satisfy. The
// migration run needs a single connection, because the lock it takes is that
// connection's.
type migrator interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// applyOne runs a migration and records it in the same transaction, so a
// crash mid-migration leaves neither half applied.
func applyOne(ctx context.Context, conn migrator, version, body string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s: %w", version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, body); err != nil {
		return fmt.Errorf("apply %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx,
		`insert into schema_migrations (version) values ($1)
		 on conflict (version) do nothing`, version); err != nil {
		return fmt.Errorf("record %s: %w", version, err)
	}
	return tx.Commit(ctx)
}

func appliedVersions(ctx context.Context, conn migrator) (map[string]struct{}, error) {
	rows, err := conn.Query(ctx, `select version from schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	out := map[string]struct{}{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = struct{}{}
	}
	return out, rows.Err()
}
