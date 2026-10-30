// Package testdb gives every Postgres-backed test package a database of its
// own on the server TEST_DATABASE_URL points at.
//
// The suites assume they own the rows they count: they truncate, sweep and
// tally whole tables. Sharing one database, `go test ./internal/...` ran them
// concurrently and they erased each other's fixtures — failures that looked
// exactly like a regression in whatever branch was under test (#162). A
// database per package keeps that assumption true without serialising the run.
package testdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// maxIdent is Postgres' identifier limit (NAMEDATALEN - 1). Past it the server
// truncates silently, which would fold two long names back into one database.
const maxIdent = 63

// Name derives the database a test package uses from the base database name
// and the package's path below internal/. The result holds only [a-z0-9_], so
// it is an identifier whatever the inputs were called; it is still quoted
// wherever it reaches SQL.
//
// The readable part is lossy: sanitising folds "channel/connect" and
// "channel-connect" together, joining folds ("agents_test", "x") and
// ("agents", "test_x"), and truncation folds long names. So every name ends in
// a suffix hashed from the original pair, which keeps those apart; hashing the
// sanitised text instead would inherit every collision it is meant to break.
func Name(base, pkg string) string {
	sum := sha256.Sum256([]byte(base + "\x00" + pkg))
	suffix := "_" + hex.EncodeToString(sum[:4])
	readable := sanitize(base + "_" + pkg)
	if len(readable) > maxIdent-len(suffix) {
		readable = readable[:maxIdent-len(suffix)]
	}
	return readable + suffix
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}

// WithDatabase returns dsn pointing at database name, everything else kept.
func WithDatabase(dsn, name string) (string, error) {
	u, err := parseURL(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	u.RawPath = ""
	return u.String(), nil
}

func parseURL(dsn string) (*url.URL, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse TEST_DATABASE_URL: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return nil, fmt.Errorf("TEST_DATABASE_URL must be a postgres:// URL, got scheme %q", u.Scheme)
	}
	return u, nil
}

// Ensure makes sure the database for pkg exists on the server base points at,
// and returns a DSN for it. It connects through the base database itself —
// the one database the caller already proved exists — rather than assuming
// the server has a maintenance database this role may reach.
func Ensure(ctx context.Context, base, pkg string) (string, error) {
	u, err := parseURL(base)
	if err != nil {
		return "", err
	}
	name := Name(strings.TrimPrefix(u.Path, "/"), pkg)
	dsn, err := WithDatabase(base, name)
	if err != nil {
		return "", err
	}
	if err := create(ctx, base, name); err != nil {
		return "", err
	}
	return dsn, nil
}

func create(ctx context.Context, base, name string) error {
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		return fmt.Errorf("connect to the base test database: %w", err)
	}
	defer conn.Close(ctx)

	var exists bool
	if err := conn.QueryRow(ctx,
		`select exists (select 1 from pg_database where datname = $1)`, name,
	).Scan(&exists); err != nil {
		return fmt.Errorf("look up database %s: %w", name, err)
	}
	if exists {
		return nil
	}
	_, err = conn.Exec(ctx, "create database "+pgx.Identifier{name}.Sanitize())
	if err == nil || lostTheRace(err) {
		return nil
	}
	return fmt.Errorf("create database %s: %w", name, err)
}

// lostTheRace reports whether another caller created the database between our
// lookup and our CREATE. Postgres says so either as duplicate_database or, when
// both inserts reach the catalogue together, as a unique violation on it.
func lostTheRace(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "42P04" || pgErr.Code == "23505"
}
