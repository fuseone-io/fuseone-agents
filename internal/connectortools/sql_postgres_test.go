package connectortools

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresSQL_describeBelievesTheDatabase(t *testing.T) {
	t.Parallel()

	fx := newSQLPostgresFixture(t)
	session := fx.open(t, fx.reader)
	defer closeSQLSession(t, session)

	withParameter := fmt.Sprintf("select id from %s.records where id = $1", fx.schema)
	count, err := session.Describe(context.Background(), withParameter)
	if err != nil {
		t.Fatalf("Describe parameter: %v", err)
	}
	if count != 1 {
		t.Fatalf("parameters = %d, want 1", count)
	}

	insideLiteral := `select '$1'`
	count, err = session.Describe(context.Background(), insideLiteral)
	if err != nil {
		t.Fatalf("Describe literal: %v", err)
	}
	if count != 0 {
		t.Fatalf("literal parameters = %d, want 0", count)
	}
}

func TestPostgresSQL_queryIsReadOnlyEvenForAWriter(t *testing.T) {
	t.Parallel()

	fx := newSQLPostgresFixture(t)
	session := fx.open(t, fx.writer)
	defer closeSQLSession(t, session)

	query := fmt.Sprintf("insert into %s.records (note) values ($1) returning id", fx.schema)
	err := session.Query(context.Background(), query, []any{"must-not-be-written"}, &collectingSQLSink{})
	if err == nil {
		t.Fatal("a write ran inside the governed read transaction")
	}

	var count int
	check := fmt.Sprintf("select count(*) from %s.records where note = $1", fx.schema)
	if err := fx.admin.QueryRow(context.Background(), check, "must-not-be-written").Scan(&count); err != nil {
		t.Fatalf("count written rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("written rows = %d, want 0", count)
	}
}

func TestPostgresSQL_streamsRowsUntilTheSinkStops(t *testing.T) {
	t.Parallel()

	fx := newSQLPostgresFixture(t)
	session := fx.open(t, fx.reader)
	defer closeSQLSession(t, session)

	stop := errors.New("test sink is full")
	sink := &collectingSQLSink{stopAfter: 2, stop: stop}
	err := session.Query(
		context.Background(), "select n from generate_series(1, 100) as n", nil, sink)
	if !errors.Is(err, stop) {
		t.Fatalf("Query error = %v, want sink error", err)
	}
	if len(sink.rows) != 2 {
		t.Fatalf("rows delivered = %d, want 2", len(sink.rows))
	}
	if len(sink.columns) != 1 || sink.columns[0] != "n" {
		t.Fatalf("columns = %#v, want [n]", sink.columns)
	}
}

func TestPostgresSQL_preservesIdentifiersAndExactNumbersInGovernedJSON(t *testing.T) {
	t.Parallel()

	fx := newSQLPostgresFixture(t)
	session := fx.open(t, fx.reader)
	defer closeSQLSession(t, session)

	sink := &collectingSQLSink{}
	err := session.Query(context.Background(), `select
		'9045b230-79cb-4c82-8d16-0f1ff1ab8b7d'::uuid,
		9007199254740993.123456789::numeric,
		1152921504606846975::int8,
		'192.0.2.1/24'::inet,
		'192.0.2.0/24'::cidr,
		'\x0102'::bytea,
		'{"ok":true,"n":2}'::jsonb,
		array['9045b230-79cb-4c82-8d16-0f1ff1ab8b7d'::uuid],
		array[9007199254740993.123456789::numeric]`, nil, sink)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(sink.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(sink.rows))
	}
	want := `[` +
		`"9045b230-79cb-4c82-8d16-0f1ff1ab8b7d",` +
		`"9007199254740993.123456789",` +
		`"1152921504606846975",` +
		`"192.0.2.1/24","192.0.2.0/24","AQI=",` +
		`{"n":2,"ok":true},` +
		`["9045b230-79cb-4c82-8d16-0f1ff1ab8b7d"],` +
		`["9007199254740993.123456789"]]`
	if got := string(sink.rows[0]); got != want {
		t.Fatalf("row = %s, want %s", got, want)
	}
}

func TestPostgresSQL_theServerEnforcesTheExecutionBudget(t *testing.T) {
	t.Parallel()

	fx := newSQLPostgresFixture(t)
	session := fx.openWithTimeout(t, fx.reader, 100*time.Millisecond)
	defer closeSQLSession(t, session)

	started := time.Now()
	err := session.Query(
		context.Background(), "select pg_sleep(2)", nil, &collectingSQLSink{})
	if err == nil {
		t.Fatal("the database ran past its server-side statement timeout")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("server timeout returned after %s, want under 1s", elapsed)
	}
}

func TestPostgresConnectionConfig_removesFallbacksAndCarriesTheServerBudget(t *testing.T) {
	t.Parallel()

	parsed, err := pgx.ParseConfig("postgres://localhost/postgres?sslmode=prefer")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if len(parsed.Fallbacks) == 0 {
		t.Fatal("test fixture has no plaintext fallback to remove")
	}
	configurePostgresConnection(parsed, SQLConfig{
		Host: "db.internal", Port: 5432, Database: "app",
	}, Credential{username: "reader", password: "secret"}, nil, 1500*time.Microsecond)
	if len(parsed.Fallbacks) != 0 {
		t.Fatalf("fallbacks = %d, want none", len(parsed.Fallbacks))
	}
	if got := parsed.RuntimeParams["statement_timeout"]; got != "2ms" {
		t.Fatalf("statement_timeout = %q, want 2ms", got)
	}
	if got := parsed.RuntimeParams["idle_in_transaction_session_timeout"]; got != "2ms" {
		t.Fatalf("idle timeout = %q, want 2ms", got)
	}
	for name, want := range map[string]string{
		"datestyle": "ISO, MDY", "intervalstyle": "iso_8601",
		"timezone": "UTC", "extra_float_digits": "3", "bytea_output": "hex",
	} {
		if got := parsed.RuntimeParams[name]; got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestPostgresSQL_verifiesTheServerCertificate(t *testing.T) {
	t.Parallel()

	fx := newSQLPostgresFixture(t)
	wrongRoots := x509.NewCertPool()
	executor := NewPostgresSQLExecutor(wrongRoots)
	if _, err := executor.Open(
		context.Background(), fx.config, fx.reader, time.Second,
	); err == nil {
		t.Fatal("a server outside the configured trust roots was accepted")
	}

	session := fx.open(t, fx.reader)
	closeSQLSession(t, session)
}

func TestPostgresSQL_closeReleasesTheTransportAfterItsContextExpires(t *testing.T) {
	t.Parallel()

	fx := newSQLPostgresFixture(t)
	session := fx.open(t, fx.reader)
	postgres, ok := session.(*postgresSQLSession)
	if !ok {
		t.Fatalf("session = %T, want postgres session", session)
	}
	pid := postgres.conn.PgConn().PID()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = session.Close(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for {
		var alive bool
		if err := fx.admin.QueryRow(
			context.Background(), "select exists(select 1 from pg_stat_activity where pid = $1)", pid,
		).Scan(&alive); err != nil {
			t.Fatalf("inspect transport: %v", err)
		}
		if !alive {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("backend pid %d survived Close with an expired context", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type collectingSQLSink struct {
	columns   []string
	rows      []json.RawMessage
	stopAfter int
	stop      error
}

func (s *collectingSQLSink) Columns(names []string) error {
	s.columns = append([]string(nil), names...)
	return nil
}

func (s *collectingSQLSink) Row(row json.RawMessage) error {
	if s.stopAfter > 0 && len(s.rows) == s.stopAfter {
		return s.stop
	}
	s.rows = append(s.rows, append(json.RawMessage(nil), row...))
	return nil
}

type sqlPostgresFixture struct {
	admin    *pgx.Conn
	executor *PostgresSQLExecutor
	config   SQLConfig
	reader   Credential
	writer   Credential
	schema   string
}

func newSQLPostgresFixture(t *testing.T) sqlPostgresFixture {
	t.Helper()
	readerDSN := requiredSQLTestDSN(t, "TEST_SQL_DATABASE_URL")
	writerDSN := requiredSQLTestDSN(t, "TEST_SQL_WRITER_DATABASE_URL")
	adminDSN := requiredSQLTestDSN(t, "TEST_SQL_ADMIN_DATABASE_URL")

	readerConfig := parseSQLTestConfig(t, readerDSN)
	writerConfig := parseSQLTestConfig(t, writerDSN)
	admin, err := pgx.Connect(context.Background(), adminDSN)
	if err != nil {
		t.Fatalf("connect SQL test administrator: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })

	schema := sqlTestSchema(t.Name())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(context.Background(), "create schema "+quoted); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "drop schema if exists "+quoted+" cascade")
	})
	setupSQLFixture(t, admin, quoted)

	return sqlPostgresFixture{
		admin: admin, executor: NewPostgresSQLExecutor(readerConfig.TLSConfig.RootCAs),
		config: SQLConfig{
			Driver: SQLDriverPostgres, Host: readerConfig.Host,
			Port: int(readerConfig.Port), Database: readerConfig.Database,
		},
		reader: Credential{username: readerConfig.User, password: readerConfig.Password},
		writer: Credential{username: writerConfig.User, password: writerConfig.Password},
		schema: schema,
	}
}

func setupSQLFixture(t *testing.T, admin *pgx.Conn, schema string) {
	t.Helper()
	statements := []string{
		"create table " + schema + ".records (id bigint generated always as identity primary key, note text not null)",
		"grant usage on schema " + schema + " to sqlconn, sqlwriter",
		"grant select on all tables in schema " + schema + " to sqlconn",
		"grant select, insert, update, delete on all tables in schema " + schema + " to sqlwriter",
		"grant usage, select on all sequences in schema " + schema + " to sqlwriter",
	}
	for _, statement := range statements {
		if _, err := admin.Exec(context.Background(), statement); err != nil {
			t.Fatalf("prepare SQL fixture: %v", err)
		}
	}
}

func (f sqlPostgresFixture) open(t *testing.T, credential Credential) SQLSession {
	return f.openWithTimeout(t, credential, time.Second)
}

func (f sqlPostgresFixture) openWithTimeout(
	t *testing.T, credential Credential, timeout time.Duration,
) SQLSession {
	t.Helper()
	session, err := f.executor.Open(context.Background(), f.config, credential, timeout)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return session
}

func closeSQLSession(t *testing.T, session SQLSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func requiredSQLTestDSN(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value != "" {
		return value
	}
	if os.Getenv("REQUIRE_DATABASE") != "" {
		t.Fatalf("REQUIRE_DATABASE is set but %s is empty", name)
	}
	t.Skip("SQL connector Postgres is not configured")
	return ""
}

func parseSQLTestConfig(t *testing.T, dsn string) *pgx.ConnConfig {
	t.Helper()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse SQL test DSN: %v", err)
	}
	return cfg
}

func sqlTestSchema(name string) string {
	sum := sha256.Sum256([]byte(name))
	return "sqltest_" + strings.ToLower(hex.EncodeToString(sum[:6]))
}
