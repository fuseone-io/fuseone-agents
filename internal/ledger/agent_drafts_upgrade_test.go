package ledger_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/ledger"
)

/*
0113 only adds (NT-018 1b): two tables of its own, and nothing the previous
binary reads or writes changes shape. So 0.117.0 keeps running against the
upgraded schema, and rolling back to it leaves the drafts unread.

Checked against the schema the previous release left: every column, trigger
and constraint of every table that existed at 0112 is the same after the
upgrade, and a row the previous binary writes still goes in.
*/
func TestMigrate_upgradeTo0113_addsTheDraftTablesAndChangesNothingElse(t *testing.T) {
	pool := freshDatabase(t, "ledger/upgrade_0113")
	ctx := t.Context()
	if err := ledger.MigrateThrough(ctx, pool, "0112_inbox_remembers_the_receiving_scope"); err != nil {
		t.Fatalf("migrate through 0112: %v", err)
	}
	before := schemaShape(ctx, t, pool)
	if _, err := pool.Exec(ctx, `
		insert into agent_state (agent_id, paused, changed_at) values ('triage', true, now())`); err != nil {
		t.Fatalf("the previous binary's write before the upgrade: %v", err)
	}

	// Through 0113 alone: a later migration's tables are its own test's.
	if err := ledger.MigrateThrough(ctx, pool, "0113_agent_drafts"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	after := schemaShape(ctx, t, pool)
	var added []string
	for _, line := range after {
		if !slices.Contains(before, line) {
			added = append(added, line)
		}
	}
	for _, line := range before {
		if !slices.Contains(after, line) {
			t.Errorf("0113 changed what was there: %s", line)
		}
	}
	for _, line := range added {
		if !strings.HasPrefix(line, "agent_drafts ") && !strings.HasPrefix(line, "agent_draft_trail ") {
			t.Errorf("0113 added outside its own tables: %s", line)
		}
	}
	if len(added) == 0 {
		t.Fatal("0113 added nothing; the draft tables are missing")
	}
	if _, err := pool.Exec(ctx, `
		update agent_state set paused = false, changed_at = now() where agent_id = 'triage'`); err != nil {
		t.Errorf("the previous binary's write after the upgrade: %v", err)
	}
}

// schemaShape is every column, trigger and constraint in the schema, one line
// each, prefixed by its table.
func schemaShape(ctx context.Context, t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		select table_name || ' column ' || column_name || ' ' || data_type || ' ' ||
		       coalesce(column_default, '') || ' ' || is_nullable
		from information_schema.columns where table_schema = 'public'
		union all
		select event_object_table || ' trigger ' || trigger_name || ' ' || event_manipulation || ' ' || action_statement
		from information_schema.triggers where trigger_schema = 'public'
		union all
		select c.conrelid::regclass::text || ' constraint ' || c.conname || ' ' || pg_get_constraintdef(c.oid)
		from pg_constraint c join pg_namespace n on n.oid = c.connamespace
		where n.nspname = 'public'`)
	if err != nil {
		t.Fatalf("read the schema: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the schema: %v", err)
	}
	return out
}
