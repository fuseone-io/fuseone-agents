package testdb

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/ledger"
)

// DSN returns the DSN of the calling test package's own database, creating it
// on first use, or "" when TEST_DATABASE_URL is unset. Callers keep deciding
// what an absent database means for them — skip, fail, or fall back to a fake.
//
// The database is not migrated here: a suite that proves migrations needs it
// as it is. Pool is the helper for everybody else.
func DSN(t testing.TB) string {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		return ""
	}
	pkg, err := packagePath()
	if err != nil {
		t.Fatalf("name the test package: %v", err)
	}
	dsn, err := Ensure(t.Context(), base, pkg)
	if err != nil {
		t.Fatalf("prepare the package database: %v", err)
	}
	return dsn
}

// Pool opens a migrated pool on the calling package's own database, closed
// when the test ends. Without TEST_DATABASE_URL the test skips, or fails when
// REQUIRE_DATABASE says a skip would be a silent hole in the run.
func Pool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dsn := Required(t)
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

// Required is DSN for a suite that cannot run without the database: it skips
// when TEST_DATABASE_URL is unset, and fails instead under REQUIRE_DATABASE.
func Required(t testing.TB) string {
	t.Helper()
	dsn := DSN(t)
	if dsn != "" {
		return dsn
	}
	if os.Getenv("REQUIRE_DATABASE") != "" {
		t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
	}
	t.Skip("TEST_DATABASE_URL is unset; skipping a Postgres suite")
	return ""
}

// packagePath names the package under test by its directory below the module
// root, internal/ dropped: "ledger", "channel/connect". go test runs every
// test binary in its package's source directory, so the working directory is
// the package — no caller has to remember to pass its own name.
func packagePath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}
	root, err := moduleRoot(dir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", fmt.Errorf("package below the module root: %w", err)
	}
	return strings.TrimPrefix(filepath.ToSlash(rel), "internal/"), nil
}

var errNoModule = errors.New("no go.mod above the test's directory")

func moduleRoot(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errNoModule
		}
		dir = parent
	}
}
