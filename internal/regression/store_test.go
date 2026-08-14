package regression_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
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
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is unset; skipping the regression suite")
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
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
