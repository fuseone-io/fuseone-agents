package ledger

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strings"

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
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	files, err := fs.Glob(migrationFS, "sql/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	slices.Sort(files)

	// Bootstrap: the table that records progress cannot record its own
	// creation, so it is created idempotently before anything is checked.
	if _, err := pool.Exec(ctx, `
		create table if not exists schema_migrations (
			version    text        not null primary key,
			applied_at timestamptz not null default now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, pool)
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
		if err := applyOne(ctx, pool, version, string(body)); err != nil {
			return err
		}
	}
	return nil
}

// applyOne runs a migration and records it in the same transaction, so a
// crash mid-migration leaves neither half applied.
func applyOne(ctx context.Context, pool *pgxpool.Pool, version, body string) error {
	tx, err := pool.Begin(ctx)
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

func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[string]struct{}, error) {
	rows, err := pool.Query(ctx, `select version from schema_migrations`)
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
