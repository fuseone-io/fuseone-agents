package testdb

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/ledger"
)

// DeclareWriter is what a fixture says, inside its transaction, to write the working
// set of a corpus directly. Everything else that writes it records a version.
const DeclareWriter = `select set_config('fuseone.dataset_writer', 'version-aware', true);`

// Isolated opens a migrated database of the test's own, dropped when the test ends.
//
// For what cannot be undone in the package's database: rows that refuse to be
// deleted, or a state a migration has to find.
func Isolated(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool := empty(t)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// IsolatedThrough is Isolated stopped at version, for a test of what comes after it.
// ledger.Migrate finishes the job.
func IsolatedThrough(t testing.TB, version string) *pgxpool.Pool {
	t.Helper()
	pool := empty(t)
	if err := ledger.MigrateThrough(t.Context(), pool, version); err != nil {
		t.Fatalf("migrate through %s: %v", version, err)
	}
	return pool
}

func empty(t testing.TB) *pgxpool.Pool {
	t.Helper()
	Required(t)
	base := os.Getenv("TEST_DATABASE_URL")
	ctx := t.Context()

	name := Name("isolated", t.Name())
	if err := drop(ctx, base, name); err != nil {
		t.Fatalf("drop a leftover database: %v", err)
	}
	if err := create(ctx, base, name); err != nil {
		t.Fatalf("create the database: %v", err)
	}
	t.Cleanup(func() { _ = drop(context.Background(), base, name) })

	dsn, err := WithDatabase(base, name)
	if err != nil {
		t.Fatalf("name the database: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func drop(ctx context.Context, base, name string) error {
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, "drop database if exists "+pgx.Identifier{name}.Sanitize()+" with (force)")
	return err
}
