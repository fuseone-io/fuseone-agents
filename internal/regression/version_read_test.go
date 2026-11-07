package regression_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/simulate"
)

func TestPinned_answersTheLatestVersionWithItsCases(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	if err := store.Record(t.Context(), aCase("reg-3", never("x"))); err != nil {
		t.Fatalf("Record: %v", err)
	}

	version, cases, err := store.Pinned(t.Context(), "suporte")
	if err != nil {
		t.Fatalf("Pinned: %v", err)
	}
	if version != 2 || len(cases) != 3 {
		t.Errorf("version = %d, cases = %+v, want version 2 with three", version, cases)
	}
}

func TestPinned_anAgentWithNoCorpus_answersNothing(t *testing.T) {
	store := regression.NewStore(activatedCorpus(t))

	version, cases, err := store.Pinned(t.Context(), "another")
	if err != nil || version != 0 || len(cases) != 0 {
		t.Errorf("version = %d, cases = %+v, err = %v", version, cases, err)
	}
}

// What a battery ran stays what it is read against, whatever happens to the corpus.
func TestAtVersion_answersTheCorpusAsItStood(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	ctx := t.Context()

	changed := aCase("reg-1", never("changed since"))
	changed.InputRef = copyOne
	if err := store.Record(ctx, changed); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(ctx, aCase("reg-3", never("added since"))); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Delete(ctx, "suporte", "reg-2", "usr_ana"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	cases, err := store.AtVersion(ctx, "suporte", 1)
	if err != nil {
		t.Fatalf("AtVersion: %v", err)
	}
	if len(cases) != 2 || cases[0].ID != "reg-1" || cases[1].ID != "reg-2" {
		t.Fatalf("cases = %+v, want reg-1 and reg-2", cases)
	}
	if cases[0].Expectations[0].Value != "ana@example.com" {
		t.Errorf("reg-1 expects %+v, want what it expected then", cases[0].Expectations)
	}
	if !cases[1].Erased {
		t.Errorf("reg-2 = %+v, want its input still erased", cases[1])
	}
}

// Erasure wins over a pin: a version keeps naming a case, never saying what it said.
func TestAtVersion_contentErasedSince_readsAsErased(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	ctx := t.Context()
	eraseAs096(t, pool, []string{copyOne}, []string{"run-1"})

	cases, err := store.AtVersion(ctx, "suporte", 1)
	if err != nil {
		t.Fatalf("AtVersion: %v", err)
	}
	if !cases[0].Erased || len(cases[0].Expectations) != 0 {
		t.Errorf("reg-1 = %+v, want it erased with nothing to check", cases[0])
	}
}

func TestAtVersion_aVersionThatDoesNotExist_isRefused(t *testing.T) {
	store := regression.NewStore(activatedCorpus(t))

	if _, err := store.AtVersion(t.Context(), "suporte", 99); !errors.Is(err, regression.ErrNoSuchVersion) {
		t.Errorf("err = %v, want it refused: an empty corpus would read as nothing broke", err)
	}
}

// NT-015's pinning test: adding a case after a battery leaves its numbers unchanged.
func TestBattery_readAgainstItsVersion_doesNotMoveWhenTheCorpusGrows(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)
	ctx := t.Context()
	report := simulate.Report{ID: "sim-1", DatasetVersion: 1, Cases: []simulate.Case{{ID: "reg-1"}}}
	numbers := func(cases []domain.RegressionCase) (int, int, int) {
		out := simulate.Battery(report, cases)
		return out.Held, out.Broken, out.Erased
	}

	before, err := store.AtVersion(ctx, "suporte", 1)
	if err != nil {
		t.Fatalf("AtVersion: %v", err)
	}
	h1, b1, e1 := numbers(before)
	if err := store.Record(ctx, aCase("reg-3", never("x"))); err != nil {
		t.Fatalf("Record: %v", err)
	}
	after, err := store.AtVersion(ctx, "suporte", 1)
	if err != nil {
		t.Fatalf("AtVersion: %v", err)
	}

	if h2, b2, e2 := numbers(after); h1 != h2 || b1 != b2 || e1 != e2 {
		t.Errorf("held/broken/erased moved from %d/%d/%d to %d/%d/%d", h1, b1, e1, h2, b2, e2)
	}
}

func TestOccurrences_nameTheVersionTheyCameFrom(t *testing.T) {
	pool := activatedCorpus(t)
	store := regression.NewStore(pool)

	corpus, err := regression.NewCorpus(store, contentOf(pool)).Occurrences(t.Context(), "suporte")
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if corpus.Version != 1 || len(corpus.Cases) != 1 || corpus.Cases[0].ID != "reg-1" {
		t.Errorf("corpus = %+v, want version 1 with the case that can run", corpus)
	}
}

func contentOf(pool *pgxpool.Pool) *ledger.Content { return ledger.NewContent(pool) }
