package regression_test

import (
	"context"
	"errors"
	"os"
	"testing"

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
	if _, err := pool.Exec(t.Context(), `delete from regression_cases`); err != nil {
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
	if err := store.Delete(ctx, "suporte", "reg-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ := store.List(ctx, "suporte")
	if len(got) != 1 || got[0].ID != "reg-2" {
		t.Errorf("cases = %v", ids(got))
	}
}

// Deleting a case left its copy behind with nothing pointing at it, so no
// erasure could ever find it again.
func TestDelete_erasesTheCopyOfTheOccurrence(t *testing.T) {
	store := storeFor(t)
	pool := testdb.Pool(t)
	ctx := context.Background()
	content := ledger.NewContent(pool)

	ref, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"email":"ana@example.com"}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	withRef := aCase("reg-1", domain.Expectation{Kind: domain.ExpectAsks})
	withRef.InputRef = ref
	if err := store.Record(ctx, withRef); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if err := store.Delete(ctx, "suporte", "reg-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := content.Get(ctx, ref); !errors.Is(err, ledger.ErrErased) {
		t.Fatalf("copy after delete: err = %v, want erased", err)
	}
}

// Identical input shares one reference, so the copy is another case's too.
func TestDelete_keepsACopyAnotherCaseStillNames(t *testing.T) {
	store := storeFor(t)
	pool := testdb.Pool(t)
	ctx := context.Background()
	content := ledger.NewContent(pool)

	ref, err := content.PutFor(ctx, "regression", "suporte", 1, []byte(`{"shared":true}`))
	if err != nil {
		t.Fatalf("PutFor: %v", err)
	}
	for _, id := range []string{"reg-1", "reg-2"} {
		c := aCase(id, domain.Expectation{Kind: domain.ExpectAsks})
		c.InputRef = ref
		if err := store.Record(ctx, c); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	if err := store.Delete(ctx, "suporte", "reg-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := content.Get(ctx, ref); err != nil {
		t.Fatalf("copy another case names: %v", err)
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

	migration, err := os.ReadFile("../ledger/sql/0097_erase_orphaned_corpus_copies.sql")
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
