package regression_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/testdb"
)

// Release A cannot turn versions on, and NT-015's rollout rests on what happens when
// a later release does. So the activation runs here, on a database of its own, and
// everything an older binary can still do is run against it as the SQL it really sends.

const (
	copyOne = "regression://suporte/1/one"
	copyTwo = "regression://suporte/1/two"
)

// seedCorpus is a corpus as it stands before activation: one case that runs, and one
// whose input is gone, with what 0102 left of it.
func seedCorpus(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		insert into run_content (ref, owner_kind, run_id, seq, digest, bytes, full_bytes, created_at)
		values ('regression://suporte/1/one', 'regression', 'suporte', 1, 'digest-one', 'one', 3,
		        '2026-01-02 00:00:00+00');
		insert into run_content (ref, owner_kind, run_id, seq, digest, bytes, full_bytes, created_at,
		                         erased_at, erased_reason)
		values ('regression://suporte/1/two', 'regression', 'suporte', 1, 'digest-two', null, 3,
		        '2026-01-03 00:00:00+00', '2026-02-01 00:00:00+00', 'subject');
		insert into regression_cases (agent_id, case_id, company_id, area_id, input_ref, from_run, expectations)
		values ('suporte', 'reg-1', 'acme', 'cx', 'regression://suporte/1/one', 'run-1',
		        '[{"kind":"never_calls","value":"ana@example.com"}]'),
		       ('suporte', 'reg-2', 'acme', 'cx', 'regression://suporte/1/two', 'run-2', '[]');
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func activate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), testdb.ActivateDatasetVersions); err != nil {
		t.Fatalf("activate: %v", err)
	}
}

func activatedCorpus(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.Isolated(t)
	seedCorpus(t, pool)
	activate(t, pool)
	return pool
}

func TestVersionRows_updateAndDelete_refusedByTheDatabase(t *testing.T) {
	pool := activatedCorpus(t)

	for _, statement := range []string{
		`update dataset_versions set digest = 'other'`,
		`delete from dataset_versions`,
		`update dataset_version_cases set input_ref = 'other'`,
		`delete from dataset_version_cases`,
	} {
		_, err := pool.Exec(t.Context(), statement)
		if err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s: err = %v, want it refused as append-only", statement, err)
		}
	}
}

// A new text column in an immutable table has to be argued for here. Anything
// readable in one of these rows outlives every erasure.
func TestVersionTables_holdOnlyIdsReferencesAndDigests(t *testing.T) {
	pool := testdb.Pool(t)
	allowed := map[string]bool{
		"dataset_versions.dataset_id": true, "dataset_versions.digest": true,
		"dataset_versions.created_by":               true,
		"dataset_version_cases.dataset_id":          true,
		"dataset_version_cases.case_id":             true,
		"dataset_version_cases.input_ref":           true,
		"dataset_version_cases.input_digest":        true,
		"dataset_version_cases.expectations_ref":    true,
		"dataset_version_cases.expectations_digest": true,
		"dataset_version_cases.from_run":            true,
	}

	rows, err := pool.Query(t.Context(), `
		select table_name || '.' || column_name from information_schema.columns
		where table_schema = current_schema()
		  and table_name in ('dataset_versions', 'dataset_version_cases')
		  and data_type not in ('bigint', 'timestamp with time zone')`)
	if err != nil {
		t.Fatalf("read the columns: %v", err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen++
		if !allowed[column] {
			t.Errorf("%s is not on the list of ids, references and digests", column)
		}
	}
	if seen != len(allowed) {
		t.Errorf("saw %d columns, the list names %d", seen, len(allowed))
	}
}

func TestActivation_convertsExpectationsToContentNamingTheCase(t *testing.T) {
	pool := activatedCorpus(t)

	var body, kind string
	var created time.Time
	if err := pool.QueryRow(t.Context(), `
		select convert_from(rc.bytes, 'UTF8'), rc.owner_kind, rc.created_at
		from regression_cases c join run_content rc on rc.ref = c.expectations_ref
		where c.case_id = 'reg-1'`).Scan(&body, &kind, &created); err != nil {
		t.Fatalf("read the converted object: %v", err)
	}
	if !strings.Contains(body, "reg-1") || !strings.Contains(body, "ana@example.com") {
		t.Errorf("object = %s, want the case's name and its expectations", body)
	}
	if kind != "expectations" {
		t.Errorf("owner kind = %s", kind)
	}
	if want := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC); !created.Equal(want) {
		t.Errorf("created at = %s, want the age of the input, %s", created, want)
	}
}

func TestActivation_aCaseWhoseInputIsGone_becomesATombstone(t *testing.T) {
	pool := activatedCorpus(t)

	var bytes []byte
	var erased *time.Time
	var reason string
	if err := pool.QueryRow(t.Context(), `
		select rc.bytes, rc.erased_at, rc.erased_reason
		from regression_cases c join run_content rc on rc.ref = c.expectations_ref
		where c.case_id = 'reg-2'`).Scan(&bytes, &erased, &reason); err != nil {
		t.Fatalf("read the converted object: %v", err)
	}
	if bytes != nil || erased == nil || reason != "subject" {
		t.Errorf("bytes = %q, erased at %v, reason %q, want the input's tombstone", bytes, erased, reason)
	}
}

func TestActivation_writesVersionOneFromTheCorpusAsItStands(t *testing.T) {
	pool := activatedCorpus(t)

	var versions, cases int
	var active bool
	if err := pool.QueryRow(t.Context(), `
		select (select count(*) from dataset_versions where dataset_id = 'default:suporte' and version = 1),
		       (select count(*) from dataset_version_cases where dataset_id = 'default:suporte' and version = 1),
		       (select active from dataset_versioning)`).Scan(&versions, &cases, &active); err != nil {
		t.Fatalf("read: %v", err)
	}
	if versions != 1 || cases != 2 || !active {
		t.Errorf("versions = %d, cases = %d, active = %v", versions, cases, active)
	}
}

/*
The decisive case of the rollout: a writer that does not know versions exist.

Each statement is what 0.95.0 sends. It must fail and change nothing, because a
working set no version records is what activation exists to prevent, and the Delete
also takes content in the same statement.
*/
func TestActivation_aWriterThatDoesNotKnowVersions_failsWithoutChangingTheWorkingSet(t *testing.T) {
	const upsert = `
		insert into regression_cases (
			agent_id, case_id, company_id, area_id,
			input_ref, expectations, from_run, note, created_by, model, effort
		) values ('suporte', %s, 'acme', 'cx', $1, '[{"kind":"asks"}]', 'run-9', '', 'usr', '', '')
		on conflict (agent_id, case_id) do update set
			input_ref = excluded.input_ref,
			expectations = excluded.expectations,
			from_run = excluded.from_run,
			note = excluded.note,
			model = excluded.model,
			effort = excluded.effort`
	const deleteCase = `
		with gone as (
			delete from regression_cases where agent_id = 'suporte' and case_id = 'reg-1'
			returning input_ref
		)
		update run_content rc set bytes = null, erased_at = now(), erased_reason = 'case_deleted'
		where rc.erased_at is null and rc.ref in (select input_ref from gone)`

	pool := activatedCorpus(t)
	for name, statement := range map[string]struct {
		sql  string
		args []any
	}{
		"record a new case":   {fmt.Sprintf(upsert, "'reg-9'"), []any{copyOne}},
		"record over a case":  {fmt.Sprintf(upsert, "'reg-1'"), []any{copyTwo}},
		"delete a case":       {deleteCase, nil},
		"change expectations": {`update regression_cases set expectations = '[{"kind":"asks"}]' where case_id = 'reg-1'`, nil},
		"empty a live case":   {`update regression_cases set expectations = '[]' where case_id = 'reg-1'`, nil},
	} {
		t.Run(name, func(t *testing.T) {
			before := workingSet(t, pool)
			_, err := pool.Exec(t.Context(), statement.sql, statement.args...)
			if err == nil || !strings.Contains(err.Error(), "does not record dataset versions") {
				t.Fatalf("err = %v, want it refused by the trigger", err)
			}
			if after := workingSet(t, pool); after != before {
				t.Errorf("working set moved:\n%s\n%s", before, after)
			}
		})
	}
}

// workingSet is everything a version covers, and whether the copy is still there.
func workingSet(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var out string
	if err := pool.QueryRow(t.Context(), `
		select coalesce(string_agg(
			c.case_id || '|' || c.input_ref || '|' || c.expectations::text || '|' || c.from_run || '|' ||
			coalesce(i.erased_at::text, 'live'), E'\n' order by c.case_id), '')
		from regression_cases c left join run_content i on i.ref = c.input_ref`).Scan(&out); err != nil {
		t.Fatalf("read the working set: %v", err)
	}
	return out
}

// What 0.95.0 sends to erase. Erasure cannot wait for a switch, so it has to keep
// working, and it has to reach what activation made.
func TestActivation_theErasureOfAnOlderBinary_stillWorksAndTakesTheObject(t *testing.T) {
	pool := activatedCorpus(t)
	ctx := t.Context()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `
		update run_content set bytes = null, erased_at = now(), erased_reason = $2
		where ref = any($1::text[]) and erased_at is null`, []string{copyOne}, "subject"); err != nil {
		t.Fatalf("take the copy: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		update regression_cases set note = '', expectations = '[]'::jsonb
		where input_ref = any($1::text[]) or from_run = any($2::text[])`,
		[]string{copyOne}, []string{"run-1"}); err != nil {
		t.Fatalf("clear what the correction says: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var column string
	var bytes []byte
	var erased *time.Time
	if err := pool.QueryRow(ctx, `
		select c.expectations::text, rc.bytes, rc.erased_at
		from regression_cases c join run_content rc on rc.ref = c.expectations_ref
		where c.case_id = 'reg-1'`).Scan(&column, &bytes, &erased); err != nil {
		t.Fatalf("read: %v", err)
	}
	if column != "[]" || bytes != nil || erased == nil {
		t.Errorf("column = %s, bytes = %q, erased at %v, want nothing readable", column, bytes, erased)
	}
}

// Retention takes the same path. It must reach the object through the version rows,
// because a case deleted since is still named by the version that checked it.
func TestActivation_retention_takesTheObjectOfACaseNoLongerInTheWorkingSet(t *testing.T) {
	pool := activatedCorpus(t)
	ctx := t.Context()

	var ref string
	if err := pool.QueryRow(ctx, `
		select expectations_ref from regression_cases where case_id = 'reg-1'`).Scan(&ref); err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		begin;
		select set_config('fuseone.dataset_writer', 'version-aware', true);
		delete from regression_cases where case_id = 'reg-1';
		commit`); err != nil {
		t.Fatalf("delete as a writer that knows: %v", err)
	}
	// The input only, as a retention whose window reaches the input and not the object.
	if _, err := pool.Exec(ctx, `
		update run_content set bytes = null, erased_at = now(), erased_reason = 'retention'
		where ref = $1`, copyOne); err != nil {
		t.Fatalf("retention: %v", err)
	}

	var erased *time.Time
	var reason string
	if err := pool.QueryRow(ctx, `
		select erased_at, erased_reason from run_content where ref = $1`, ref).Scan(&erased, &reason); err != nil {
		t.Fatalf("read: %v", err)
	}
	if erased == nil || reason != "retention" {
		t.Errorf("erased at %v, reason %q, want it gone with the input", erased, reason)
	}
}

// An erasure in flight when activation starts. Seen by neither side, its expectations
// would become readable content after it reported success.
func TestActivation_waitsForAnErasureInFlight(t *testing.T) {
	pool := testdb.Isolated(t)
	seedCorpus(t, pool)
	ctx := t.Context()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `
		update run_content set bytes = null, erased_at = now(), erased_reason = 'subject'
		where ref = $1`, copyOne); err != nil {
		t.Fatalf("take the copy: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := pool.Exec(ctx, testdb.ActivateDatasetVersions)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("activation did not wait: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("activate: %v", err)
	}

	var readable int
	if err := pool.QueryRow(ctx, `
		select count(*) from run_content
		where owner_kind = 'expectations' and bytes is not null
		  and convert_from(bytes, 'UTF8') like '%ana@example.com%'`).Scan(&readable); err != nil {
		t.Fatalf("read: %v", err)
	}
	if readable != 0 {
		t.Errorf("%d readable objects hold what the erasure took", readable)
	}
}
