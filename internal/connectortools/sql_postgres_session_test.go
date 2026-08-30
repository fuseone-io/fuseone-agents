package connectortools

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestPostgresSQL_sessionOwnsTheTextRepresentation(t *testing.T) {
	t.Parallel()

	fx := newSQLPostgresFixture(t)
	credential := hostilePostgresTextRole(t, fx)
	session := fx.open(t, credential)
	defer closeSQLSession(t, session)

	sink := &collectingSQLSink{}
	err := session.Query(context.Background(), `select
		current_setting('DateStyle'),
		current_setting('IntervalStyle'),
		current_setting('TimeZone'),
		current_setting('extra_float_digits'),
		current_setting('bytea_output'),
		'\x0102'::bytea,
		'2026-08-30'::date,
		'1 day 2 hours'::interval,
		'2026-08-30 20:42:32+00'::timestamptz`, nil, sink)
	if err != nil {
		t.Fatalf("Query with hostile role defaults: %v", err)
	}
	if len(sink.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(sink.rows))
	}
	var got []string
	if err := json.Unmarshal(sink.rows[0], &got); err != nil {
		t.Fatalf("decode governed row: %v", err)
	}
	want := []string{
		"ISO, MDY", "iso_8601", "UTC", "3", "hex", "AQI=",
		"2026-08-30", "P1DT2H", "2026-08-30 20:42:32+00",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("row = %#v, want connector-owned text %#v", got, want)
	}
}

func hostilePostgresTextRole(t *testing.T, fx sqlPostgresFixture) Credential {
	t.Helper()
	role := "sqlfmt_" + fx.schema[len("sqltest_"):]
	password := "session-format-test"
	quotedRole := pgx.Identifier{role}.Sanitize()
	quotedDatabase := pgx.Identifier{fx.config.Database}.Sanitize()
	if _, err := fx.admin.Exec(
		context.Background(), "create role "+quotedRole+" login password 'session-format-test'",
	); err != nil {
		t.Fatalf("create hostile role: %v", err)
	}
	t.Cleanup(func() {
		_, _ = fx.admin.Exec(context.Background(), "drop role if exists "+quotedRole)
	})
	statements := []string{
		"grant sqlconn to " + quotedRole,
		"alter role " + quotedRole + " in database " + quotedDatabase + " set bytea_output to 'escape'",
		"alter role " + quotedRole + " in database " + quotedDatabase + " set DateStyle to 'German, DMY'",
		"alter role " + quotedRole + " in database " + quotedDatabase + " set IntervalStyle to 'postgres_verbose'",
		"alter role " + quotedRole + " in database " + quotedDatabase + " set TimeZone to 'America/Sao_Paulo'",
		"alter role " + quotedRole + " in database " + quotedDatabase + " set extra_float_digits to '0'",
	}
	for _, statement := range statements {
		if _, err := fx.admin.Exec(context.Background(), statement); err != nil {
			t.Fatalf("prepare hostile role defaults: %v", err)
		}
	}
	return Credential{username: role, password: password}
}
