package connectortools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestPostgresSQL_unstructuredTypesUsePostgresTextNotDriverValues(t *testing.T) {
	t.Parallel()

	fx := newSQLPostgresFixture(t)
	typeName := pgx.Identifier{fx.schema, "review_state"}.Sanitize()
	if _, err := fx.admin.Exec(
		context.Background(), "create type "+typeName+" as enum ('ready')",
	); err != nil {
		t.Fatalf("create custom type: %v", err)
	}

	session := fx.open(t, fx.reader)
	defer closeSQLSession(t, session)

	sink := &collectingSQLSink{}
	query := fmt.Sprintf(`select
		'12:34:56.789'::time,
		'12:34:56+02'::timetz,
		'[1,10)'::int4range,
		'<a>x</a>'::xml,
		to_tsvector('simple', 'hello world'),
		array['08:00:2b:01:02:03:04:05'::macaddr8],
		'ready'::%s`, typeName)
	if err := session.Query(context.Background(), query, nil, sink); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(sink.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(sink.rows))
	}
	var got []string
	if err := json.Unmarshal(sink.rows[0], &got); err != nil {
		t.Fatalf("decode governed row: %v", err)
	}
	want := []string{
		"12:34:56.789", "12:34:56+02", "[1,10)", "<a>x</a>",
		"'hello':1 'world':2", "{08:00:2b:01:02:03:04:05}", "ready",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("row = %#v, want %#v", got, want)
	}
}
