package testdb_test

import (
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/testdb"
)

// The whole point of the helper: two suites running at once must not count
// each other's rows, so two packages can never land in one database.
func TestName_twoPackages_getTwoDatabases(t *testing.T) {
	t.Parallel()
	ledger := testdb.Name("agents_test", "ledger")
	channel := testdb.Name("agents_test", "channel")
	if ledger == channel {
		t.Fatalf("both packages share %q", ledger)
	}
	if ledger != "agents_test_ledger" {
		t.Errorf("ledger database = %q, want agents_test_ledger", ledger)
	}
}

// A nested package is its own suite, distinct from its parent.
func TestName_nestedPackage_isDistinctFromItsParent(t *testing.T) {
	t.Parallel()
	nested := testdb.Name("agents_test", "channel/connect")
	if nested != "agents_test_channel_connect" {
		t.Errorf("nested database = %q, want agents_test_channel_connect", nested)
	}
	if nested == testdb.Name("agents_test", "channel") {
		t.Error("a nested package shares its parent's database")
	}
}

// The name ends up in CREATE DATABASE. Whatever a directory or a base URL is
// called, what comes out is an identifier and nothing else.
func TestName_hostileInput_yieldsOnlySafeCharacters(t *testing.T) {
	t.Parallel()
	cases := []struct{ base, pkg string }{
		{"agents_test", `x"; drop database agents; --`},
		{"agents-test", "Channel/Connect"},
		{"agents_test", "ünïcode/päckage"},
		{`a"b`, "pkg"},
	}
	for _, c := range cases {
		got := testdb.Name(c.base, c.pkg)
		if strings.Trim(got, "abcdefghijklmnopqrstuvwxyz0123456789_") != "" {
			t.Errorf("Name(%q, %q) = %q carries characters outside [a-z0-9_]", c.base, c.pkg, got)
		}
	}
}

// Postgres silently truncates identifiers at 63 bytes, so two long names that
// differ only past the limit would quietly become one database again.
func TestName_longNames_fitAndStayDistinct(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("deeply/nested/", 6)
	a := testdb.Name("agents_test", long+"alpha")
	b := testdb.Name("agents_test", long+"beta")
	if len(a) > 63 || len(b) > 63 {
		t.Fatalf("names exceed 63 bytes: %d, %d", len(a), len(b))
	}
	if a == b {
		t.Fatalf("two long packages collapse into %q", a)
	}
}

func TestWithDatabase_replacesOnlyTheDatabase(t *testing.T) {
	t.Parallel()
	got, err := testdb.WithDatabase(
		"postgres://u:p@127.0.0.1:5433/agents_test?sslmode=disable", "agents_test_ledger")
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	want := "postgres://u:p@127.0.0.1:5433/agents_test_ledger?sslmode=disable"
	if got != want {
		t.Errorf("dsn = %q, want %q", got, want)
	}
}

// Every package's first test races its siblings to create the database, and
// in a parallel run the other test binaries race too. Losing that race is the
// ordinary case and must not fail anybody.
func TestEnsure_concurrentCallers_allGetTheSameExistingDatabase(t *testing.T) {
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is unset; creating a database needs a server")
	}
	const pkg = `testdb/race"; drop database x; --`
	dropDerived(t, base, pkg)
	const callers = 6
	dsns := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dsns[i], errs[i] = testdb.Ensure(t.Context(), base, pkg)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
	}

	conn, err := pgx.Connect(t.Context(), dsns[0])
	if err != nil {
		t.Fatalf("connect to the derived database: %v", err)
	}
	defer conn.Close(t.Context())
	var name string
	if err := conn.QueryRow(t.Context(), `select current_database()`).Scan(&name); err != nil {
		t.Fatalf("read the database name: %v", err)
	}
	if !strings.Contains(name, "_testdb_race_") {
		t.Errorf("connected to %q, not the derived database", name)
	}
}

// dropDerived removes the database a previous run left, so every run races on
// a database nobody has created yet — the case the tolerance exists for.
func dropDerived(t *testing.T, base, pkg string) {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}
	name := testdb.Name(strings.TrimPrefix(u.Path, "/"), pkg)
	conn, err := pgx.Connect(t.Context(), base)
	if err != nil {
		t.Fatalf("connect to the base database: %v", err)
	}
	defer conn.Close(t.Context())
	if _, err := conn.Exec(t.Context(),
		"drop database if exists "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("drop %s: %v", name, err)
	}
}
