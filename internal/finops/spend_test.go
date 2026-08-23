package finops_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/finops"
	"github.com/fuseone/agents/internal/ledger"
)

func spendPoolFor(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the finops suite")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		truncate run_steps, runs, planning_spend, planning_spend_cursor`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool
}

func seedSpendCursor(t *testing.T, pool *pgxpool.Pool, at time.Time) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		insert into planning_spend_cursor (id, scanned_at, scanned_run_id, scanned_seq)
		values (true, $1, '', 0)
		on conflict (id) do update set
			scanned_at = excluded.scanned_at,
			scanned_run_id = excluded.scanned_run_id,
			scanned_seq = excluded.scanned_seq`, at.UTC()); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}
}

func appendPlanned(
	t *testing.T, store *ledger.Postgres, id domain.RunID, at time.Time,
	cost domain.Cost, planned domain.PlannedPayload,
) {
	t.Helper()
	scope := domain.Scope{Company: "acme", Area: "platform"}
	if _, err := store.Append(t.Context(), domain.Step{
		RunID: id, Kind: domain.StepRunStarted, Scope: scope,
		AgentID: "triage", VersionID: "v1", At: at.UTC(),
	}); err != nil {
		t.Fatalf("Append start %s: %v", id, err)
	}
	body, err := json.Marshal(planned)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := store.Append(t.Context(), domain.Step{
		RunID: id, Kind: domain.StepPlanned, Scope: scope,
		AgentID: "triage", VersionID: "v1", At: at.Add(time.Second).UTC(),
		Cost: cost, Payload: body,
	}); err != nil {
		t.Fatalf("Append planned %s: %v", id, err)
	}
}

func TestSpend_projectsEachPlanningCallOnce(t *testing.T) {
	pool := spendPoolFor(t)
	store := ledger.NewPostgres(pool)
	spend := finops.NewSpend(pool)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	seedSpendCursor(t, pool, base)

	appendPlanned(t, store, "run-a", base.Add(time.Second),
		domain.Cost{Micros: 900, InputTokens: 100},
		domain.PlannedPayload{Provider: "anthropic", Model: "claude-haiku-4-5"})

	n, err := spend.Project(t.Context(), 100)
	if err != nil || n != 1 {
		t.Fatalf("Project = %d, %v; want one row", n, err)
	}

	// Idempotent by (run_id, seq): a second pass over the same step must not
	// double the money. A sweep that can only run once is a sweep that cannot
	// be reprocessed, which was the reason for choosing a projection.
	if n, err := spend.Project(t.Context(), 100); err != nil || n != 0 {
		t.Fatalf("second Project = %d, %v; want nothing new", n, err)
	}

	var rows, micros int64
	var model string
	if err := pool.QueryRow(t.Context(), `
		select count(*), coalesce(sum(cost_micros), 0), coalesce(max(model), '')
		from planning_spend`).Scan(&rows, &micros, &model); err != nil {
		t.Fatalf("read: %v", err)
	}
	if rows != 1 || micros != 900 || model != "claude-haiku-4-5" {
		t.Fatalf("rows=%d micros=%d model=%q, want the call recorded once", rows, micros, model)
	}
}

func TestSpend_skipsAPlanningStepThatNamesNoModel(t *testing.T) {
	pool := spendPoolFor(t)
	store := ledger.NewPostgres(pool)
	spend := finops.NewSpend(pool)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	seedSpendCursor(t, pool, base)

	// Recorded before the pair was written. Attributing it would put spend
	// against a model the step never named; failing on it would stop the sweep
	// at the oldest row in the installation and never reach today's.
	appendPlanned(t, store, "run-old", base.Add(time.Second),
		domain.Cost{Micros: 500}, domain.PlannedPayload{Node: "triage"})
	appendPlanned(t, store, "run-new", base.Add(2*time.Second),
		domain.Cost{Micros: 700}, domain.PlannedPayload{Provider: "openai", Model: "gpt-4o-mini"})

	if _, err := spend.Project(t.Context(), 100); err != nil {
		t.Fatalf("Project: %v", err)
	}

	var runs []string
	rowsIter, err := pool.Query(t.Context(), `select run_id from planning_spend order by run_id`)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	defer rowsIter.Close()
	for rowsIter.Next() {
		var id string
		if err := rowsIter.Scan(&id); err != nil {
			t.Fatal(err)
		}
		runs = append(runs, id)
	}
	if len(runs) != 1 || runs[0] != "run-new" {
		t.Fatalf("projected %v, want only the step that named its model", runs)
	}
}

func TestSpend_cursorPassesAStepItSkipped(t *testing.T) {
	pool := spendPoolFor(t)
	store := ledger.NewPostgres(pool)
	spend := finops.NewSpend(pool)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	seedSpendCursor(t, pool, base)

	// A step with no pair, then one with. A batch of one reads only the first,
	// writes nothing, and must still move past it: a cursor that advanced only
	// over what it wrote would rediscover this row every minute for ever and
	// never reach the second.
	appendPlanned(t, store, "run-old", base.Add(time.Second),
		domain.Cost{Micros: 500}, domain.PlannedPayload{Node: "triage"})
	appendPlanned(t, store, "run-new", base.Add(2*time.Second),
		domain.Cost{Micros: 700}, domain.PlannedPayload{Provider: "openai", Model: "gpt-4o-mini"})

	if n, err := spend.Project(t.Context(), 1); err != nil || n != 0 {
		t.Fatalf("first Project = %d, %v; want nothing written", n, err)
	}
	if n, err := spend.Project(t.Context(), 1); err != nil || n != 1 {
		t.Fatalf("second Project = %d, %v; want the step behind the skipped one", n, err)
	}
}

func TestSpend_halfAPairIsNotProjected(t *testing.T) {
	pool := spendPoolFor(t)
	store := ledger.NewPostgres(pool)
	spend := finops.NewSpend(pool)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	seedSpendCursor(t, pool, base)

	appendPlanned(t, store, "run-no-provider", base.Add(time.Second),
		domain.Cost{Micros: 500}, domain.PlannedPayload{Model: "gpt-4o-mini"})

	if n, err := spend.Project(t.Context(), 10); err != nil || n != 0 {
		t.Fatalf("Project = %d, %v; want a model with no provider left out", n, err)
	}
}

func TestSpend_resumesWhereTheBatchStopped(t *testing.T) {
	pool := spendPoolFor(t)
	store := ledger.NewPostgres(pool)
	spend := finops.NewSpend(pool)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	seedSpendCursor(t, pool, base)

	for i, id := range []domain.RunID{"run-1", "run-2", "run-3"} {
		appendPlanned(t, store, id, base.Add(time.Duration(i+1)*time.Second),
			domain.Cost{Micros: 100}, domain.PlannedPayload{Provider: "anthropic", Model: "m"})
	}

	// A batch smaller than the backlog: the next pass resumes from where this
	// one stopped rather than starting over or skipping ahead.
	if n, err := spend.Project(t.Context(), 2); err != nil || n != 2 {
		t.Fatalf("Project = %d, %v; want the batch size", n, err)
	}
	if n, err := spend.Project(t.Context(), 2); err != nil || n != 1 {
		t.Fatalf("second Project = %d, %v; want the remainder", n, err)
	}

	var rows int64
	if err := pool.QueryRow(t.Context(),
		`select count(*) from planning_spend`).Scan(&rows); err != nil {
		t.Fatalf("read: %v", err)
	}
	if rows != 3 {
		t.Fatalf("rows = %d, want every planning call projected exactly once", rows)
	}
}

func TestSpend_aggregatesByModelAndSaysWhatIsUnpriced(t *testing.T) {
	pool := spendPoolFor(t)
	store := ledger.NewPostgres(pool)
	spend := finops.NewSpend(pool)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	seedSpendCursor(t, pool, base)

	priced := domain.ModelPriceUse{Status: domain.ModelPriceConfigured}
	missing := domain.ModelPriceUse{Status: domain.ModelPriceMissing}

	appendPlanned(t, store, "run-1", base.Add(time.Second),
		domain.Cost{Micros: 900, InputTokens: 100},
		domain.PlannedPayload{Provider: "anthropic", Model: "opus", Price: &priced})
	appendPlanned(t, store, "run-2", base.Add(2*time.Second),
		domain.Cost{Micros: 100, InputTokens: 20},
		domain.PlannedPayload{Provider: "anthropic", Model: "opus", Price: &priced})
	// Same provider, no rate: its tokens are real and its money is not.
	appendPlanned(t, store, "run-3", base.Add(3*time.Second),
		domain.Cost{Micros: 0, InputTokens: 5_000},
		domain.PlannedPayload{Provider: "anthropic", Model: "haiku", Price: &missing})

	if _, err := spend.Project(t.Context(), 100); err != nil {
		t.Fatalf("Project: %v", err)
	}

	got, err := spend.ByModel(t.Context(), base, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("ByModel: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d buckets, want one per model", len(got))
	}

	// Ordered by spend, so the expensive one is the first thing read.
	if got[0].Model != "opus" || got[0].Micros != 1000 || got[0].Calls != 2 {
		t.Fatalf("first bucket = %+v, want opus summed across its calls", got[0])
	}
	// The unpriced one is not hidden and not counted as free: it carries its
	// tokens and says the money is missing, so a total nobody can trust says so.
	if got[1].Model != "haiku" || got[1].InputTokens != 5_000 || got[1].Unpriced != 1 {
		t.Fatalf("second bucket = %+v, want haiku with its tokens and unpriced flagged", got[1])
	}
}
