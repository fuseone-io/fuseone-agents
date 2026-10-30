package ledger_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The fallback filters probe run_steps for a model_fallback on every run they
weigh. Without an index on that kind the probe is an anti-join over a sequential
scan of every partition, and the Trust Center runs it several times a page while
the ledger only grows.

Sequential scans are switched off for the plan: on a test-sized table the
planner may rightly prefer one, and the question is not whether it chose the
index today but whether an index the filter can use exists at all. The assertion
names the partition indexes attached to run_steps_model_fallback_run_idx, so a
plan that walks some other index with a filter does not pass.
*/
func TestFallbackFilters_probeThePartialIndex(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `set enable_seqscan = off`); err != nil {
		t.Fatalf("set: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `reset enable_seqscan`) })

	indexes := attachedIndexes(t, conn.Conn(), "run_steps_model_fallback_run_idx")

	for name, query := range map[string]string{
		"steps": `select agent_id from run_steps where kind = 'approval_decided' and ` +
			ledger.DeclaredPairStepsFilter,
		"runs": `select agent_id from runs where ` + ledger.DeclaredPairRunsFilter,
	} {
		plan := explain(t, conn.Conn(), query)
		if !usesAny(plan, indexes) {
			t.Errorf("%s: the fallback probe uses none of %v:\n%s", name, indexes, plan)
		}
	}
}

// attachedIndexes is the parent index and every partition index attached to
// it. An empty list fails the test: the index the filters need is not there.
func attachedIndexes(t *testing.T, conn *pgx.Conn, parent string) []string {
	t.Helper()
	rows, err := conn.Query(context.Background(), `
		select c.relname from pg_inherits i
		join pg_class c on c.oid = i.inhrelid
		join pg_class p on p.oid = i.inhparent
		where p.relname = $1`, parent)
	if err != nil {
		t.Fatalf("read the partition indexes: %v", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(names) == 0 {
		t.Fatalf("no partition index is attached to %s", parent)
	}
	return append(names, parent)
}

func explain(t *testing.T, conn *pgx.Conn, query string) string {
	t.Helper()
	rows, err := conn.Query(context.Background(), `explain `+query)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	lines, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	return strings.Join(lines, "\n")
}

func usesAny(plan string, indexes []string) bool {
	for _, index := range indexes {
		if strings.Contains(plan, " "+index+" ") {
			return true
		}
	}
	return false
}
