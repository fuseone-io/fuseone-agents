package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/vault"
)

type activatedCorpus struct {
	pool     *pgxpool.Pool
	erasures *admin.Erasures
	content  *ledger.Content
	store    *regression.Store
	copied   string
}

// activatedCase is one correction made from run-named, on a database where versions
// are on. The copy's digest is not the digest of any input the ledger recorded, as
// when an input was too large to keep whole, so only the case leads from the run to it.
func activatedCase(t *testing.T) activatedCorpus {
	t.Helper()
	pool := testdb.IsolatedThrough(t, "0103_dataset_versions")
	ctx := t.Context()
	key := make([]byte, 32)
	v, _ := vault.New(key, "test")
	content := ledger.NewContent(pool)
	store := regression.NewStore(pool)
	erasures := admin.NewErasures(pool, content,
		admin.NewRetention(pool, settings.NewStore(pool, v)), store)

	copied, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"email":"ana@example.com"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into regression_cases (agent_id, case_id, input_ref, from_run, expectations)
		values ('suporte', 'reg-1', $1, 'run-named', '[{"kind":"never_calls","value":"ana@example.com"}]')`,
		copied); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := ledger.Migrate(ctx, pool); err != nil {
		t.Fatalf("activate: %v", err)
	}
	return activatedCorpus{pool, erasures, content, store, copied}
}

func (c activatedCorpus) expectationsObject(t *testing.T) string {
	t.Helper()
	var ref string
	if err := c.pool.QueryRow(t.Context(), `
		select expectations_ref from dataset_version_cases where version = 1 and case_id = 'reg-1'`,
	).Scan(&ref); err != nil {
		t.Fatalf("read the version: %v", err)
	}
	return ref
}

func (c activatedCorpus) expectGone(t *testing.T, refs ...string) {
	t.Helper()
	for _, ref := range refs {
		if _, err := c.content.Get(t.Context(), ref); !errors.Is(err, ledger.ErrErased) {
			t.Errorf("%s: err = %v, want erased", ref, err)
		}
	}
}

// The erasure every binary sends has to get past the trigger.
func TestForSubject_afterActivation_takesALiveCase(t *testing.T) {
	c := activatedCase(t)
	object := c.expectationsObject(t)

	if _, err := c.erasures.ForSubject(t.Context(), "dpo", domain.Scope{},
		[]domain.RunID{"run-named"}, "request"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	c.expectGone(t, c.copied, object)
}

// Deleted, the case is only in the version that checked it. Nothing else knows this run
// led to that copy, and no runs row says whose corpus it is: the run may be long purged.
func TestForSubject_afterActivation_reachesACaseDeletedSinceThroughItsVersion(t *testing.T) {
	c := activatedCase(t)
	object := c.expectationsObject(t)
	ctx := t.Context()
	if err := c.store.Delete(ctx, "suporte", "reg-1", "usr_ana"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := c.erasures.ForSubject(ctx, "dpo", domain.Scope{},
		[]domain.RunID{"run-named"}, "request"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	c.expectGone(t, c.copied, object)
	var rows int
	if err := c.pool.QueryRow(ctx,
		`select count(*) from dataset_version_cases where case_id = 'reg-1'`).Scan(&rows); err != nil {
		t.Fatalf("read: %v", err)
	}
	if rows != 1 {
		t.Errorf("version rows naming the case = %d, want it still named", rows)
	}
}

// An object can outlast its window on its own. Taken while a writer keeps it, the new
// version would name a tombstone the writer never saw.
func TestSweep_waitsForAnExpectationsObjectBeingKept(t *testing.T) {
	future := time.Now().Add(6 * 365 * 24 * time.Hour)
	erasures, content := erasuresFor(t, func() time.Time { return future })
	pool := openPool(t)
	cleanCorpus(t, pool)
	ctx := context.Background()
	store := regression.NewStore(pool)

	if _, err := content.PutFor(ctx, "expectations", "suporte", 1, []byte(`{"case":"reg-1"}`)); err != nil {
		t.Fatalf("PutFor: %v", err)
	}

	entered, release, swept := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	held := make(chan error, 1)
	go func() {
		held <- store.Hold(ctx, "suporte", func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	go func() {
		_, err := erasures.Sweep(ctx)
		swept <- err
	}()
	select {
	case err := <-swept:
		t.Fatalf("the sweep did not wait for the writer: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-held; err != nil {
		t.Fatalf("Hold: %v", err)
	}
	if err := <-swept; err != nil {
		t.Fatalf("Sweep: %v", err)
	}
}

// A case whose copy was never stored: no tombstone happens, so the trigger never sees
// it. Naming its run still has to take what it says, in the column and the object.
func TestForSubject_afterActivation_takesTheObjectOfACaseWithNoCopy(t *testing.T) {
	c := activatedCase(t)
	ctx := t.Context()
	if _, err := c.pool.Exec(ctx, `
		begin; `+testdb.DeclareWriter+`
		update regression_cases set from_run = 'run-other';
		insert into regression_cases (agent_id, case_id, input_ref, from_run, expectations)
		values ('suporte', 'reg-2', 'regression://suporte/1/never-stored', 'run-named',
		        '[{"kind":"never_calls","value":"bia@example.com"}]');
		insert into run_content (ref, owner_kind, run_id, seq, digest, bytes, full_bytes)
		values ('expectations://suporte/1/of-reg-2', 'expectations', 'suporte', 1, 'of-reg-2', 'x', 1);
		update regression_cases set expectations_ref = 'expectations://suporte/1/of-reg-2'
		where case_id = 'reg-2';
		insert into dataset_versions (dataset_id, version, digest) values ('default:suporte', 2, 'two');
		insert into dataset_version_cases
			(dataset_id, version, case_id, input_ref, input_digest, expectations_ref, expectations_digest, from_run)
		values ('default:suporte', 2, 'reg-2', 'regression://suporte/1/never-stored', 'none',
		        'expectations://suporte/1/of-reg-2', 'of-reg-2', 'run-named');
		commit`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := c.erasures.ForSubject(ctx, "dpo", domain.Scope{},
		[]domain.RunID{"run-named"}, "request"); err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	c.expectGone(t, "expectations://suporte/1/of-reg-2")
	var column string
	if err := c.pool.QueryRow(ctx,
		`select expectations::text from regression_cases where case_id = 'reg-2'`).Scan(&column); err != nil {
		t.Fatalf("read: %v", err)
	}
	if column != "[]" {
		t.Errorf("column = %s, want it empty", column)
	}
}

func (c activatedCorpus) detailOf(t *testing.T, action string) int {
	t.Helper()
	var objects int
	if err := c.pool.QueryRow(t.Context(), `
		select (detail->>'objects')::int from admin_events
		where action = $1 order by at desc limit 1`, action).Scan(&objects); err != nil {
		t.Fatalf("read the event: %v", err)
	}
	return objects
}

// The event is how a DPO proves what went, so an object the trigger took on the way
// has to be in it. A copy and its object are two.
func TestForSubject_afterActivation_countsTheObjectItTook(t *testing.T) {
	c := activatedCase(t)

	erased, err := c.erasures.ForSubject(t.Context(), "dpo", domain.Scope{},
		[]domain.RunID{"run-named"}, "request")
	if err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	if erased != 2 {
		t.Errorf("erased = %d, want the copy and the object", erased)
	}
	if got := c.detailOf(t, "content.erased"); got != 2 {
		t.Errorf("detail.objects = %d, want 2", got)
	}
}

// Retention reaches the copy and not the object, which is younger: re-recorded
// expectations are. The object goes with its input, and it counts.
func TestSweep_afterActivation_countsAYoungerObjectItTook(t *testing.T) {
	c := activatedCase(t)
	ctx := t.Context()
	object := c.expectationsObject(t)
	if _, err := c.pool.Exec(ctx, `
		update run_content set created_at = '2000-01-01 00:00:00+00' where ref = $1;`, c.copied); err != nil {
		t.Fatalf("age the copy: %v", err)
	}
	if _, err := c.pool.Exec(ctx, `update run_content set created_at = now() where ref = $1`, object); err != nil {
		t.Fatalf("freshen the object: %v", err)
	}

	erased, err := c.erasures.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	c.expectGone(t, c.copied, object)
	if erased != 2 {
		t.Errorf("erased = %d, want the copy and the object", erased)
	}
	if got := c.detailOf(t, "content.expired"); got != 2 {
		t.Errorf("detail.objects = %d, want 2", got)
	}
}

// A correction made from another run shares the copy. Only the copy leads to its object.
func TestForSubject_afterActivation_countsTheObjectOfACorrectionSharingTheCopy(t *testing.T) {
	c := activatedCase(t)
	ctx := t.Context()
	if err := c.store.Record(ctx, domain.RegressionCase{
		ID: "reg-later", Agent: "suporte", InputRef: c.copied, FromRun: "run-later",
		Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "x"}},
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	erased, err := c.erasures.ForSubject(ctx, "dpo", domain.Scope{},
		[]domain.RunID{"run-named"}, "request")
	if err != nil {
		t.Fatalf("ForSubject: %v", err)
	}

	if erased != 3 {
		t.Errorf("erased = %d, want the copy and both objects", erased)
	}
}
