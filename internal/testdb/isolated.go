package testdb

import (
	"context"
	_ "embed"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/ledger"
)

// ActivateDatasetVersions is what the migration after release A will run. Until that
// release it exists only for tests, and it moves to the migrations then.
//
//go:embed activate_dataset_versions.sql
var ActivateDatasetVersions string

// Isolated opens a migrated database of the test's own, dropped when the test ends.
//
// For what cannot be undone in the package's database: a trigger that outlives the
// test, or rows that refuse to be deleted.
func Isolated(t testing.TB) *pgxpool.Pool {
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
	if err := ledger.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
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
