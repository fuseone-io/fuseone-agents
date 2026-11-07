package regression_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
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

// The corpus is the only thing standing between "we corrected this once" and
// "we correct it again every version". What it must never do is quietly stop
// checking, because a safety net that fails silently keeps reporting green.

func storeFor(t *testing.T) *regression.Store {
	t.Helper()
	pool := poolFor(t)
	if _, err := pool.Exec(t.Context(), `begin; `+testdb.DeclareWriter+` delete from regression_cases; commit; delete from run_content`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	return regression.NewStore(pool)
}

func poolFor(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.Pool(t)
	return pool
}

func aCase(id string, expectations ...domain.Expectation) domain.RegressionCase {
	return domain.RegressionCase{
		ID: id, Agent: "suporte", Scope: domain.Scope{Company: "acme", Area: "cx"},
		InputRef: "regression://suporte/1/abc", Expectations: expectations,
		FromRun: "run_1", CreatedBy: "usr_ana",
	}
}

func TestRecord_keepsTheExpectationsItWasGiven(t *testing.T) {
	store := storeFor(t)
	ctx := context.Background()

	want := aCase("reg-1",
		domain.Expectation{Kind: domain.ExpectNeverCalls, Step: "Responder", Value: "crm.refund"},
		domain.Expectation{Kind: domain.ExpectAsks},
	)
	if err := store.Record(ctx, want); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := store.List(ctx, "suporte")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || len(got[0].Expectations) != 2 {
		t.Fatalf("cases = %+v", got)
	}
	if got[0].Expectations[0].Step != "Responder" {
		// FU-13: a correction is anchored to a step, and losing the anchor
		// would make it fail whenever any other step changed.
		t.Errorf("expectation = %+v, want its step kept", got[0].Expectations[0])
	}
	if got[0].InputRef != want.InputRef || got[0].FromRun != "run_1" {
		t.Errorf("case = %+v, want the occurrence and its provenance", got[0])
	}
}

func TestRecord_withNoExpectation_isRefused(t *testing.T) {
	store := storeFor(t)

	// A correction nothing can check is a note. Notes are welcome; they are
	// not a regression case, and letting one in makes a battery report a case
	// as passing that was never checked.
	if err := store.Record(context.Background(), aCase("reg-empty")); err == nil {
		t.Fatal("want a refusal")
	}
}

func TestList_isStableAcrossReads(t *testing.T) {
	store := storeFor(t)
	ctx := context.Background()

	for _, id := range []string{"reg-3", "reg-1", "reg-2"} {
		if err := store.Record(ctx, aCase(id,
			domain.Expectation{Kind: domain.ExpectSettles, Value: "finished"})); err != nil {
			t.Fatalf("Record %s: %v", id, err)
		}
	}

	first, err := store.List(ctx, "suporte")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	second, _ := store.List(ctx, "suporte")

	// Two reports of the same battery have to be comparable case by case, and
	// they are not if case three is case one on the second read.
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("order changed between reads: %v then %v", ids(first), ids(second))
		}
	}
	if len(first) != 3 {
		t.Errorf("cases = %d", len(first))
	}
}

func ids(cases []domain.RegressionCase) []string {
	out := make([]string, 0, len(cases))
	for _, c := range cases {
		out = append(out, c.ID)
	}
	return out
}

func TestDelete_removesOnlyThatCase(t *testing.T) {
	store := storeFor(t)
	ctx := context.Background()

	for _, id := range []string{"reg-1", "reg-2"} {
		if err := store.Record(ctx, aCase(id,
			domain.Expectation{Kind: domain.ExpectSettles, Value: "finished"})); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	if err := store.Delete(ctx, "suporte", "reg-1", "usr_ana"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ := store.List(ctx, "suporte")
	if len(got) != 1 || got[0].ID != "reg-2" {
		t.Errorf("cases = %v", ids(got))
	}
}

// The copies a delete used to leave behind are taken once, and accounted for.
func TestOrphanedCopiesMigration_takesOnlyCopiesNoCaseNames(t *testing.T) {
	store := storeFor(t)
	pool := testdb.Pool(t)
	ctx := context.Background()
	content := ledger.NewContent(pool)

	orphan, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"left":"behind"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	named, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"still":"named"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	run, err := content.Put(ctx, "run-1", 1, []byte(`{"a":"run"}`))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	withRef := aCase("reg-1", domain.Expectation{Kind: domain.ExpectAsks})
	withRef.InputRef = named
	if err := store.Record(ctx, withRef); err != nil {
		t.Fatalf("Record: %v", err)
	}

	migration, err := os.ReadFile("../ledger/sql/0098_erase_orphaned_corpus_copies.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if _, err := content.Get(ctx, orphan); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("orphan: err = %v, want erased", err)
	}
	if _, err := content.Get(ctx, named); err != nil {
		t.Fatalf("a copy a case names: %v", err)
	}
	if _, err := content.Get(ctx, run); err != nil {
		t.Fatalf("a run's content: %v", err)
	}
	var accounted int
	if err := pool.QueryRow(ctx, `
		select count(*) from admin_events
		where detail->>'reason' = 'orphaned corpus copies'`).Scan(&accounted); err != nil {
		t.Fatalf("read events: %v", err)
	}
	if accounted == 0 {
		t.Fatal("the erasure left no record")
	}
}

func TestList_saysWhichCasesLostTheirInput(t *testing.T) {
	store := storeFor(t)
	pool := testdb.Pool(t)
	ctx := context.Background()
	content := ledger.NewContent(pool)

	gone, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"aged":"out"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	kept, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"still":"here"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	for id, ref := range map[string]string{"reg-gone": gone, "reg-kept": kept} {
		c := aCase(id, domain.Expectation{Kind: domain.ExpectAsks})
		c.InputRef = ref
		if err := store.Record(ctx, c); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `
		update run_content set bytes = null, erased_at = now(), erased_reason = 'retention'
		where ref = $1`, gone); err != nil {
		t.Fatalf("erase: %v", err)
	}

	listed, err := store.List(ctx, "suporte")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	erased := map[string]bool{}
	for _, c := range listed {
		erased[c.ID] = c.Erased
	}
	if !erased["reg-gone"] || erased["reg-kept"] {
		t.Fatalf("erased = %v", erased)
	}

	found, err := store.ErasedCases(ctx)
	if err != nil {
		t.Fatalf("ErasedCases: %v", err)
	}
	if len(found) != 1 || found[0].Case != "reg-gone" || found[0].Agent != "suporte" {
		t.Fatalf("erased cases = %+v", found)
	}

	occurrences, err := regression.NewCorpus(store, content).Occurrences(ctx, "suporte")
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if len(occurrences) != 1 || occurrences[0].ID != "reg-kept" {
		t.Fatalf("occurrences = %+v, want the one that can still run", occurrences)
	}
}

// With the case gone nothing else leads to the runs that replayed it, and for an
// input too large to keep whole their bytes do not match the run it came from.
func replayHolding(t *testing.T, pool *pgxpool.Pool, content *ledger.Content, input []byte) string {
	t.Helper()
	return runHolding(t, pool, content, "suporte",
		`{"trigger":"simulation","simulated":true,"simulation":"b","case":"reg-1"}`, input)
}

func runHolding(
	t *testing.T, pool *pgxpool.Pool, content *ledger.Content,
	agent domain.AgentID, started string, input []byte,
) string {
	t.Helper()
	ctx := context.Background()
	run := domain.RunID("run-holding-" + fmt.Sprint(time.Now().UnixNano()))
	if _, err := ledger.NewPostgres(pool).Append(ctx, domain.Step{
		RunID: run, Seq: 1, Kind: domain.StepRunStarted, AgentID: agent,
		Scope: domain.Scope{Company: "acme", Area: "suporte"}, IdemKey: string(run), VersionID: "v1",
		At:      time.Now(),
		Payload: []byte(started),
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	ref, err := content.Put(ctx, run, 1, input)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	return ref
}

// An input too large to keep whole: the run's digest covers all of it, while the
// copy and its replays hold and digest only what was kept. Nothing leads from the
// run to them once the case is gone, so the migration is the last chance.
func TestOrphanedCopiesMigration_takesTheReplaysOfATruncatedInput(t *testing.T) {
	storeFor(t)
	pool := testdb.Pool(t)
	ctx := context.Background()
	limited := ledger.NewContent(pool).WithLimit(64)
	whole := []byte(strings.Repeat("abcdefgh", 32))

	source, err := limited.Put(ctx, "run-source", 1, whole)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	kept, err := limited.Get(ctx, source)
	if err != nil || bytes.Equal(kept, whole) {
		t.Fatalf("kept = %q, %v; the fixture needs a truncated input", kept, err)
	}
	if _, err := limited.PutFor(ctx, "regression", "suporte", 1, kept); err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	replayed := replayHolding(t, pool, limited, kept)

	migration, err := os.ReadFile("../ledger/sql/0098_erase_orphaned_corpus_copies.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if _, err := limited.Get(ctx, replayed); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("replay of an orphaned copy: err = %v, want erased", err)
	}
}

// Identical bytes elsewhere are somebody else's record, and a real run is the
// operator's to name.
func TestOrphanedCopiesMigration_staysInsideTheAgentsRehearsals(t *testing.T) {
	storeFor(t)
	pool := testdb.Pool(t)
	ctx := context.Background()
	content := ledger.NewContent(pool)
	input := []byte(`{"email":"ana@example.com"}`)

	if _, err := content.PutFor(ctx, "regression", "suporte", 1, input); err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	real := runHolding(t, pool, content, "suporte", `{"trigger":"manual"}`, input)
	other := runHolding(t, pool, content, "vendas",
		`{"trigger":"simulation","simulated":true,"simulation":"b","case":"reg-v"}`, input)
	if replayed := replayHolding(t, pool, content, input); replayed == "" {
		t.Fatal("no replay")
	}

	migration, err := os.ReadFile("../ledger/sql/0098_erase_orphaned_corpus_copies.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply: %v", err)
	}

	for name, ref := range map[string]string{"a real run": real, "another agent's rehearsal": other} {
		if _, err := content.Get(ctx, ref); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A battery has read this case and has yet to open its runs.
func TestDelete_waitsForACopyInProgress(t *testing.T) {
	store := storeFor(t)
	ctx := context.Background()

	entered, release, deleted := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	held := make(chan error, 1)
	go func() {
		held <- store.Hold(ctx, "suporte", func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	go func() { deleted <- store.Delete(ctx, "suporte", "reg-1", "usr_ana") }()

	select {
	case err := <-deleted:
		t.Fatalf("Delete did not wait for the battery: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-held; err != nil {
		t.Fatalf("Hold: %v", err)
	}
	if err := <-deleted; err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func smallPool(t *testing.T, conns int32) *pgxpool.Pool {
	t.Helper()
	config, err := pgxpool.ParseConfig(testdb.DSN(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	config.MaxConns = conns
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Each Hold needs the pool for its own work. If the fence also sat on a pool
// connection, as many Holds as connections would each wait for one forever.
func TestHold_asManyAsThePoolHasConnections_allFinish(t *testing.T) {
	storeFor(t)
	pool := smallPool(t, 2)
	store := regression.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var inside sync.WaitGroup
	inside.Add(2)
	results := make(chan error, 2)
	for range 2 {
		go func() {
			results <- store.Hold(ctx, "suporte", func(ctx context.Context) error {
				inside.Done()
				inside.Wait()
				_, err := store.List(ctx, "suporte")
				return err
			})
		}()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("Hold: %v", err)
		}
	}
}

// The fence takes nothing from a saturated pool, so what is inside it can finish
// as soon as anybody gives a connection back.
func TestHold_withThePoolSaturated_stillEntersTheFence(t *testing.T) {
	storeFor(t)
	pool := smallPool(t, 1)
	store := regression.NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	taken, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer taken.Release()

	if err := store.Hold(ctx, "suporte", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Hold: %v", err)
	}
}

func TestHold_finishing_givesItsSlotBack(t *testing.T) {
	store := storeFor(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for range regression.FenceSlots + 1 {
		if err := store.Hold(ctx, "suporte", func(context.Context) error { return nil }); err != nil {
			t.Fatalf("Hold: %v", err)
		}
	}
}

func TestHold_withNoSlotLeft_givesUpWithItsContext(t *testing.T) {
	store := storeFor(t)
	ctx := context.Background()

	release := make(chan struct{})
	defer close(release)
	var entered sync.WaitGroup
	entered.Add(regression.FenceSlots)
	for range regression.FenceSlots {
		go func() {
			_ = store.Hold(ctx, "suporte", func(context.Context) error {
				entered.Done()
				<-release
				return nil
			})
		}()
	}
	entered.Wait()

	short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	err := store.Hold(short, "suporte", func(context.Context) error {
		t.Error("entered a fence with no slot left")
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Hold = %v, want the context's error", err)
	}
}
