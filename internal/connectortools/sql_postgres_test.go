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

func TestPostgresSQL_verifiesTheServerCertificate(t *testing.T) {
	t.Parallel()

	fx := newSQLPostgresFixture(t)
	wrongRoots := x509.NewCertPool()
	executor := NewPostgresSQLExecutor(wrongRoots)
	if _, err := executor.Open(context.Background(), fx.config, fx.reader); err == nil {
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
	t.Helper()
	session, err := f.executor.Open(context.Background(), f.config, credential)
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
