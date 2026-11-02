package ledger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationDigestBoundary = "0081_inbox_run_index"

type appliedMigration struct {
	digest       *string
	digestSource *string
}

// Migrate applies every migration that has not run yet, in filename order.
//
// A file already recorded in schema_migrations is never re-run. Its bytes are
// checked first, so an applied migration is frozen history: editing one
// refuses the installation instead of silently letting it diverge from a fresh
// one. Fix a mistake with a new file.
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

// Migrate applies migrations and sends rollout/rollback skew warnings to logger
// when one is supplied. The optional form keeps migration-only callers that do
// not have an application logger yet source-compatible.
func Migrate(ctx context.Context, pool *pgxpool.Pool, loggers ...*slog.Logger) error {
	var logger *slog.Logger
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	return migrate(ctx, pool, migrationLogger(logger))
}

func migrationLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	return migrateSet(ctx, pool, migrations, logger)
}

// migrateSet applies whichever of migrations have not run yet. Production
// always hands it the whole embedded set; only a test hands it a prefix.
func migrateSet(
	ctx context.Context, pool *pgxpool.Pool, migrations []embeddedMigration, logger *slog.Logger,
) error {
	// One connection for the whole run: a session-level lock belongs to the
	// connection that took it, and a pool would hand the unlock to another.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire for migration: %w", err)
	}
	defer conn.Release()

	// Waits rather than fails. The second process wanted the schema migrated,
	// and by the time it wakes it is.
	if err := takeMigrationLock(ctx, conn); err != nil {
		return err
	}
	defer func() {
		// Released on a fresh context: a cancelled migration must still hand
		// the lock back, or every other process waits out the connection.
		unlock, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlock, `select pg_advisory_unlock($1)`, migrationLock)
	}()

	if err := bootstrapSchemaMigrations(ctx, conn); err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}
	if err := validateApplied(migrations, applied, logger); err != nil {
		return err
	}
	if err := baselineLegacy(ctx, conn, applied); err != nil {
		return err
	}
	return applyPending(ctx, conn, migrations, applied)
}

// bootstrapSchemaMigrations creates the table that records progress. It
// cannot record its own creation, so it is created idempotently before
// anything is checked.
func bootstrapSchemaMigrations(ctx context.Context, conn migrator) error {
	if _, err := conn.Exec(ctx, `
		create table if not exists schema_migrations (
			version    text        not null primary key,
			applied_at timestamptz not null default now(),
			digest     text,
			digest_source text
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	if _, err := conn.Exec(ctx, `
		alter table schema_migrations add column if not exists digest text;
		alter table schema_migrations add column if not exists digest_source text
	`); err != nil {
		return fmt.Errorf("upgrade schema_migrations: %w", err)
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

func appliedVersions(ctx context.Context, conn migrator) (map[string]appliedMigration, error) {
	rows, err := conn.Query(ctx, `select version, digest, digest_source from schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	out := map[string]appliedMigration{}
	for rows.Next() {
		var v string
		var digest, source *string
		if err := rows.Scan(&v, &digest, &source); err != nil {
			return nil, err
		}
		out[v] = appliedMigration{digest: digest, digestSource: source}
	}
	return out, rows.Err()
}

func validateApplied(
	migrations []embeddedMigration, applied map[string]appliedMigration, logger *slog.Logger,
) error {
	known := make(map[string]embeddedMigration, len(migrations))
	for _, migration := range migrations {
		known[migration.version] = migration
	}
	appliedVersions := make([]string, 0, len(applied))
	for version := range applied {
		appliedVersions = append(appliedVersions, version)
	}
	slices.Sort(appliedVersions)
	for _, version := range appliedVersions {
		record := applied[version]
		migration, exists := known[version]
		if !exists {
			if isToleratedFutureMigration(version, migrations, applied) {
				logger.Warn("migration was recorded by a newer release; continuing for rollback compatibility", "version", version)
				continue
			}
			return fmt.Errorf("migration %s is recorded but is not present in this binary; restore the matching release or investigate the database before continuing", version)
		}
		if record.digest == nil || *record.digest == "" {
			if _, legacy := legacyMigrationDigests[version]; !legacy {
				return fmt.Errorf("migration %s has no recorded digest; this is only supported for the legacy baseline through %s", version, migrationDigestBoundary)
			}
			if migration.digest != legacyMigrationDigests[version] {
				return migrationDigestMismatch(version, legacyMigrationDigests[version], migration.digest)
			}
			continue
		}
		if *record.digest != migration.digest {
			return migrationDigestMismatch(version, *record.digest, migration.digest)
		}
	}
	return nil
}

func isToleratedFutureMigration(version string, migrations []embeddedMigration, applied map[string]appliedMigration) bool {
	unknownNumber, ok := migrationNumber(version)
	if !ok {
		return false
	}

	maxKnown := -1
	for _, migration := range migrations {
		number, ok := migrationNumber(migration.version)
		if ok && number > maxKnown {
			maxKnown = number
		}
	}
	if unknownNumber <= maxKnown {
		return false
	}

	for _, migration := range migrations {
		if _, done := applied[migration.version]; done {
			continue
		}
		number, ok := migrationNumber(migration.version)
		if !ok || number <= unknownNumber {
			return false
		}
	}
	return true
}

func migrationNumber(version string) (int, bool) {
	if len(version) < 5 || version[4] != '_' {
		return 0, false
	}
	for _, digit := range version[:4] {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	number, err := strconv.Atoi(version[:4])
	return number, err == nil
}

func migrationDigestMismatch(version, recorded, expected string) error {
	return fmt.Errorf("migration %s digest mismatch: recorded %s, expected %s; restore the canonical migration history or apply a corrective migration, never edit an applied migration", version, recorded, expected)
}

func baselineLegacy(
	ctx context.Context, conn migrator, applied map[string]appliedMigration,
) error {
	for version, record := range applied {
		if record.digest != nil && *record.digest != "" {
			continue
		}
		canonical, legacy := legacyMigrationDigests[version]
		if !legacy {
			continue
		}
		if _, err := conn.Exec(ctx,
			`update schema_migrations
			 set digest = $2, digest_source = 'baseline'
			 where version = $1 and digest is null`, version, canonical); err != nil {
			return fmt.Errorf("record baseline for %s: %w", version, err)
		}
	}
	return nil
}
