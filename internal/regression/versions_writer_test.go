package regression_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/testdb"
)

func never(value string) domain.Expectation {
	return domain.Expectation{Kind: domain.ExpectNeverCalls, Value: value}
}

func casesOf(t *testing.T, pool *pgxpool.Pool, version int) string {
	t.Helper()
	var out string
	if err := pool.QueryRow(t.Context(), `
		select coalesce(string_agg(case_id, ',' order by case_id), '')
		from dataset_version_cases
		where dataset_id = 'default:suporte' and version = $1`, version).Scan(&out); err != nil {
		t.Fatalf("read version %d: %v", version, err)
	}
	return out
}

func latestVersion(t *testing.T, pool *pgxpool.Pool) (version int, digest string) {
	t.Helper()
	if err := pool.QueryRow(t.Context(), `
		select version, digest from dataset_versions
		where dataset_id = 'default:suporte' order by version desc limit 1`).Scan(&version, &digest); err != nil {
		t.Fatalf("read the latest version: %v", err)
	}
	return version, digest
}

func TestRecord_switchOff_writesTheRowAsTodayAndNoVersion(t *testing.T) {
	pool := testdb.Isolated(t)
	store := regression.NewStore(pool)

	if err := store.Record(t.Context(), aCase("reg-1", never("x"))); err != nil {
		t.Fatalf("Record: %v", err)
	}

	var versions, objects int
	var ref *string
	if err := pool.QueryRow(t.Context(), `
		select (select count(*) from dataset_versions),
		       (select count(*) from run_content where owner_kind = 'expectations'),
		       (select expectations_ref from regression_cases where case_id = 'reg-1')`,
	).Scan(&versions, &objects, &ref); err != nil {
		t.Fatalf("read: %v", err)
	}
	if versions != 0 || objects != 0 || ref != nil {
		t.Errorf("versions = %d, objects = %d, ref = %v, want nothing new", versions, objects, ref)
	}
}

func TestRecord_afterActivation_writesANewVersionWithTheCase(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)

	if err := store.Record(t.Context(), aCase("reg-3", never("x"))); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if version, _ := latestVersion(t, pool); version != 2 {
		t.Fatalf("latest version = %d, want 2", version)
	}
	if got := casesOf(t, pool, 2); got != "reg-1,reg-2,reg-3" {
		t.Errorf("version 2 = %s", got)
	}
	if got := casesOf(t, pool, 1); got != "reg-1,reg-2" {
		t.Errorf("version 1 moved: %s", got)
	}
}

func TestRecord_changingOnlyTheExpectations_changesTheDigest(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	ctx := t.Context()

	if err := store.Record(ctx, aCase("reg-3", never("first"))); err != nil {
		t.Fatalf("Record: %v", err)
	}
	_, before := latestVersion(t, pool)
	if err := store.Record(ctx, aCase("reg-3", never("second"))); err != nil {
		t.Fatalf("Record: %v", err)
	}
	version, after := latestVersion(t, pool)
	if version != 3 || after == before {
		t.Errorf("version = %d, digest %s -> %s, want a new version with a new digest", version, before, after)
	}
}

// The digest activation wrote in SQL has to be the one a writer computes in Go, or the
// first write after activation records a version in which nothing changed.
func TestRecord_theSameCaseAgain_writesNoVersion(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)

	again := aCase("reg-1", never("ana@example.com"))
	again.InputRef, again.FromRun = copyOne, "run-1"
	if err := store.Record(t.Context(), again); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if version, _ := latestVersion(t, pool); version != 1 {
		t.Errorf("latest version = %d, want the one activation wrote", version)
	}
}

// Decoded from the object, because the column is only there for a binary being rolled
// back to.
func TestList_afterActivation_readsTheContentAndNotTheColumn(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	ctx := t.Context()

	if _, err := pool.Exec(ctx, `
		begin;
		select set_config('fuseone.dataset_writer', 'version-aware', true);
		update regression_cases set expectations = '[{"kind":"never_calls","value":"from the column"}]'
		where case_id = 'reg-1';
		commit`); err != nil {
		t.Fatalf("move the column: %v", err)
	}

	got, err := store.List(ctx, "suporte")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got[0].ID != "reg-1" || len(got[0].Expectations) != 1 || got[0].Expectations[0].Value != "ana@example.com" {
		t.Errorf("reg-1 = %+v, want what the object holds", got[0])
	}
}

// A tombstone is an answer. Falling back would bring back what somebody erased.
func TestList_aTombstonedObject_isErasedAndNeverFallsBackToTheColumn(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	ctx := t.Context()

	if _, err := pool.Exec(ctx, `
		update run_content set bytes = null, erased_at = now(), erased_reason = 'retention'
		where owner_kind = 'expectations' and ref = (
			select expectations_ref from regression_cases where case_id = 'reg-1');
		begin;
		select set_config('fuseone.dataset_writer', 'version-aware', true);
		update regression_cases set expectations = '[{"kind":"never_calls","value":"from the column"}]'
		where case_id = 'reg-1';
		commit`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := store.List(ctx, "suporte")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got[0].ID != "reg-1" || len(got[0].Expectations) != 0 || !got[0].Erased {
		t.Errorf("reg-1 = %+v, want it erased with nothing to check", got[0])
	}
}

func TestList_aReferenceToNothing_refusesTheCorpus(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	ctx := t.Context()

	if _, err := pool.Exec(ctx, `
		begin;
		select set_config('fuseone.dataset_writer', 'version-aware', true);
		update regression_cases set expectations_ref = 'expectations://suporte/1/nothing'
		where case_id = 'reg-1';
		commit`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := store.List(ctx, "suporte"); err == nil {
		t.Error("List passed over a case whose expectations are nowhere")
	}
}

// Without what it expected, a case that ran would report as checking something.
func TestOccurrences_aCaseWhoseExpectationsAreGone_isLeftOutAndNamedAsErased(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	ctx := t.Context()

	if _, err := pool.Exec(ctx, `
		update run_content set bytes = null, erased_at = now(), erased_reason = 'retention'
		where ref = (select expectations_ref from regression_cases where case_id = 'reg-1')`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := regression.NewCorpus(store, ledger.NewContent(pool)).Occurrences(ctx, "suporte")
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("occurrences = %+v, want none", got)
	}
	erased, err := store.ErasedCases(ctx)
	if err != nil {
		t.Fatalf("ErasedCases: %v", err)
	}
	if len(erased) != 2 {
		t.Errorf("erased cases = %+v, want reg-1 and reg-2", erased)
	}
}

func TestRecord_expectationsAboveTheLimit_refusedAndNothingChanges(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	before := workingSet(t, pool)

	err := store.Record(t.Context(), aCase("reg-3", never(strings.Repeat("x", domain.DefaultContentLimit))))
	if !errors.Is(err, regression.ErrExpectationsTooLarge) {
		t.Fatalf("err = %v, want it refused: a truncated object would never decode", err)
	}
	if after := workingSet(t, pool); after != before {
		t.Errorf("working set moved:\n%s\n%s", before, after)
	}
}

func TestRecord_againAfterErasure_isReadableAndAnErasedOtherCaseStaysErased(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	ctx := t.Context()

	if _, err := pool.Exec(ctx, `
		update run_content set bytes = null, erased_at = now(), erased_reason = 'subject' where ref = $1`,
		copyOne); err != nil {
		t.Fatalf("erase: %v", err)
	}
	again := aCase("reg-1", never("ana@example.com"))
	again.InputRef = copyOne
	if err := store.Record(ctx, again); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := store.List(ctx, "suporte")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got[0].Expectations) != 1 || got[1].ID != "reg-2" || !got[1].Erased {
		t.Errorf("cases = %+v", got)
	}
}

func TestDelete_afterActivation_leavesItInTheEarlierVersionWithItsData(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	ctx := t.Context()

	if err := store.Delete(ctx, "suporte", "reg-1", "usr_ana"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if got := casesOf(t, pool, 2); got != "reg-2" {
		t.Errorf("version 2 = %s", got)
	}
	if got := casesOf(t, pool, 1); got != "reg-1,reg-2" {
		t.Errorf("version 1 = %s, want it still naming what was checked", got)
	}
	var by string
	var live int
	if err := pool.QueryRow(ctx, `
		select (select created_by from dataset_versions where version = 2),
		       (select count(*) from run_content where bytes is not null and
		        (ref = $1 or ref = (select expectations_ref from dataset_version_cases
		                            where version = 1 and case_id = 'reg-1')))`, copyOne,
	).Scan(&by, &live); err != nil {
		t.Fatalf("read: %v", err)
	}
	if by != "usr_ana" || live != 2 {
		t.Errorf("by = %q, live objects = %d, want the deleter and the copy and object kept", by, live)
	}
}

func TestDelete_aCaseThatIsNotThere_writesNoVersion(t *testing.T) {
	pool := activatedCorpus(t)

	if err := regression.NewStore(pool).Delete(t.Context(), "suporte", "absent", "usr_ana"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if version, _ := latestVersion(t, pool); version != 1 {
		t.Errorf("latest version = %d", version)
	}
}

// Nothing was ever corrected on this agent, so there is no dataset to version.
func TestDelete_onAnAgentWithNoCorpus_changesNothing(t *testing.T) {
	pool := activatedCorpus(t)

	if err := regression.NewStore(pool).Delete(t.Context(), "another", "absent", "usr_ana"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// Activation holds the switch while it converts. A writer arriving then waits, and
// has to read the switch after the wait: read before it, this writer would write as if
// versions were off and be refused by the trigger.
func TestRecord_arrivingDuringActivation_waitsAndWritesAVersion(t *testing.T) {
	pool := testdb.Isolated(t)
	seedCorpus(t, pool)
	store := regression.NewStore(pool)
	ctx := t.Context()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, testdb.ActivateDatasetVersions); err != nil {
		t.Fatalf("activate: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- store.Record(ctx, aCase("reg-3", never("x"))) }()
	select {
	case err := <-done:
		t.Fatalf("the writer did not wait: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("Record: %v", err)
	}
	if got := casesOf(t, pool, 2); got != "reg-1,reg-2,reg-3" {
		t.Errorf("version 2 = %s", got)
	}
}

func TestRecord_concurrentWriters_produceContiguousVersionsWithEveryCase(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	ctx := t.Context()

	const writers = 8
	var group sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		group.Go(func() {
			errs <- store.Record(ctx, aCase(fmt.Sprintf("new-%d", i), never("x")))
		})
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	var versions, top, cases int
	if err := pool.QueryRow(ctx, `
		select count(*), max(version),
		       (select count(*) from dataset_version_cases where version = (select max(version) from dataset_versions))
		from dataset_versions`).Scan(&versions, &top, &cases); err != nil {
		t.Fatalf("read: %v", err)
	}
	if versions != writers+1 || top != writers+1 || cases != writers+2 {
		t.Errorf("versions = %d, top = %d, cases in the top one = %d", versions, top, cases)
	}
}

// An object nothing came to name would sit there readable, with nothing to lead an
// erasure to it. A note Postgres cannot store fails the write after the object.
func TestRecord_aWriteThatFails_leavesNoObjectBehind(t *testing.T) {
	pool := activatedCorpus(t)

	failing := aCase("reg-3", never("orphan@example.com"))
	failing.Note = "\x00"
	if err := regression.NewStore(pool).Record(t.Context(), failing); err == nil {
		t.Fatal("Record stored a note Postgres cannot hold")
	}

	var orphans int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from run_content
		where owner_kind = 'expectations' and convert_from(bytes, 'UTF8') like '%orphan@example.com%'`,
	).Scan(&orphans); err != nil {
		t.Fatalf("read: %v", err)
	}
	if orphans != 0 {
		t.Errorf("%d objects outlived the write that made them", orphans)
	}
}
