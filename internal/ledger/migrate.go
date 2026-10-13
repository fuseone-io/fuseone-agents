package ledger

import (
	"context"
	"crypto/sha256"
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

const migrationDigestBoundary = "0081_inbox_run_index"

type embeddedMigration struct {
	version string
	body    []byte
	digest  string
}

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

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	files, err := fs.Glob(migrationFS, "sql/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	slices.Sort(files)
	migrations, err := embeddedMigrations(migrationFS, files)
	if err != nil {
		return err
	}

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

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}
	if err := validateApplied(migrations, applied); err != nil {
		return err
	}
	if err := baselineLegacy(ctx, conn, applied); err != nil {
		return err
	}

	for _, migration := range migrations {
		version := migration.version
		if _, done := applied[version]; done {
			continue
		}
		if err := applyOne(ctx, conn, version, migration.body, migration.digest); err != nil {
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
func applyOne(ctx context.Context, conn migrator, version string, body []byte, digest string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s: %w", version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, string(body)); err != nil {
		return fmt.Errorf("apply %s: %w", version, err)
	}
	if _, err := tx.Exec(ctx,
		`insert into schema_migrations (version, digest, digest_source)
		 values ($1, $2, 'applied')
		 on conflict (version) do nothing`, version, digest); err != nil {
		return fmt.Errorf("record %s: %w", version, err)
	}
	return tx.Commit(ctx)
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

func embeddedMigrations(filesystem fs.FS, files []string) ([]embeddedMigration, error) {
	out := make([]embeddedMigration, 0, len(files))
	for _, file := range files {
		body, err := fs.ReadFile(filesystem, file)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		version := strings.TrimSuffix(strings.TrimPrefix(file, "sql/"), ".sql")
		out = append(out, embeddedMigration{
			version: version,
			body:    body,
			digest:  digestBytes(body),
		})
	}
	return out, nil
}

func digestBytes(body []byte) string {
	digest := sha256.Sum256(body)
	return fmt.Sprintf("%x", digest[:])
}

func validateApplied(
	migrations []embeddedMigration, applied map[string]appliedMigration,
) error {
	known := make(map[string]embeddedMigration, len(migrations))
	for _, migration := range migrations {
		known[migration.version] = migration
	}
	for version, record := range applied {
		migration, exists := known[version]
		if !exists {
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
