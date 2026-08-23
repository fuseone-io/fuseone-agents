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
			scanned_seq = excluded.scanned_seq,
			started_at = coalesce(planning_spend_cursor.started_at, excluded.scanned_at)`,
		at.UTC()); err != nil {
		t.Fatalf("seed cursor: %v", err)
	}
}

// call is one planning step: what it cost and what it names.
type call struct {
	cost    domain.Cost
	planned domain.PlannedPayload
}

// appendRun writes a run and every planning step under it, so a test can say
// "this agent made three calls across two runs" the way an installation does.
// A run with several turns is the ordinary case, not the exotic one.
func appendRun(
	t *testing.T, store *ledger.Postgres, id domain.RunID, agent domain.AgentID,
	at time.Time, calls ...call,
) {
	t.Helper()
	appendScopedRun(t, store, id, agent, domain.Scope{Company: "acme", Area: "platform"}, at, calls...)
}

func appendScopedRun(
	t *testing.T, store *ledger.Postgres, id domain.RunID, agent domain.AgentID,
	scope domain.Scope, at time.Time, calls ...call,
) {
	t.Helper()
	appendScopedRunWithStart(t, store, id, agent, scope, at, domain.RunStartedPayload{}, calls...)
}

func appendScopedRunWithStart(
	t *testing.T, store *ledger.Postgres, id domain.RunID, agent domain.AgentID,
	scope domain.Scope, at time.Time, start domain.RunStartedPayload, calls ...call,
) {
	t.Helper()
	startBody, err := json.Marshal(start)
	if err != nil {
		t.Fatalf("marshal start: %v", err)
	}
	if _, err := store.Append(t.Context(), domain.Step{
		RunID: id, Kind: domain.StepRunStarted, Scope: scope,
		AgentID: agent, VersionID: "v1", At: at.UTC(),
		Payload: startBody,
	}); err != nil {
		t.Fatalf("Append start %s: %v", id, err)
	}
	for i, c := range calls {
		body, err := json.Marshal(c.planned)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if _, err := store.Append(t.Context(), domain.Step{
			RunID: id, Kind: domain.StepPlanned, Scope: scope,
			AgentID: agent, VersionID: "v1",
			At:   at.Add(time.Duration(i+1) * time.Second).UTC(),
			Cost: c.cost, Payload: body,
		}); err != nil {
			t.Fatalf("Append planned %s: %v", id, err)
		}
	}
}

func appendPlanned(
	t *testing.T, store *ledger.Postgres, id domain.RunID, at time.Time,
	cost domain.Cost, planned domain.PlannedPayload,
) {
	t.Helper()
	appendRun(t, store, id, "triage", at, call{cost: cost, planned: planned})
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

func TestSpend_cursorPassesASimulatedPlanningStep(t *testing.T) {
	pool := spendPoolFor(t)
	store := ledger.NewPostgres(pool)
	spend := finops.NewSpend(pool)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	seedSpendCursor(t, pool, base)

	appendScopedRunWithStart(t, store, "run-simulated", "triage",
		domain.Scope{Company: "acme", Area: "platform"}, base.Add(time.Second),
		domain.RunStartedPayload{Trigger: "simulation", Simulated: true, Simulation: "sim-a"},
		call{
			cost:    domain.Cost{Micros: 500},
			planned: domain.PlannedPayload{Provider: "anthropic", Model: "opus"},
		})
	appendPlanned(t, store, "run-real", base.Add(2*time.Second),
		domain.Cost{Micros: 700}, domain.PlannedPayload{Provider: "anthropic", Model: "opus"})

	if n, err := spend.Project(t.Context(), 1); err != nil || n != 0 {
		t.Fatalf("first Project = %d, %v; want simulated spend skipped", n, err)
	}
	if n, err := spend.Project(t.Context(), 1); err != nil || n != 1 {
		t.Fatalf("second Project = %d, %v; want the real call behind it", n, err)
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

	got, err := spend.ByModel(t.Context(), domain.RunFilter{
		Since: base, Until: base.Add(time.Hour),
	})
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

func TestSpend_aggregatesByAgentAcrossRunsAndCarriesTheUnpriced(t *testing.T) {
	pool := spendPoolFor(t)
	store := ledger.NewPostgres(pool)
	spend := finops.NewSpend(pool)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	seedSpendCursor(t, pool, base)

	priced := domain.ModelPriceUse{Status: domain.ModelPriceConfigured}
	missing := domain.ModelPriceUse{Status: domain.ModelPriceMissing}
	spent := func(micros, in int64, model string, price *domain.ModelPriceUse) call {
		return call{
			cost:    domain.Cost{Micros: micros, InputTokens: in},
			planned: domain.PlannedPayload{Provider: "anthropic", Model: model, Price: price},
		}
	}

	// Two runs for one agent, and one of them turned twice: the agent cut has to
	// fold calls across runs, not report whichever run was last.
	appendRun(t, store, "run-a", "triage", base.Add(time.Second),
		spent(600, 100, "opus", &priced),
		spent(300, 40, "opus", &priced))
	appendRun(t, store, "run-b", "triage", base.Add(10*time.Second),
		spent(0, 5_000, "haiku", &missing))
	appendRun(t, store, "run-c", "billing", base.Add(20*time.Second),
		spent(400, 80, "opus", &priced))

	if _, err := spend.Project(t.Context(), 100); err != nil {
		t.Fatalf("Project: %v", err)
	}

	got, err := spend.ByAgent(t.Context(), domain.RunFilter{
		Since: base, Until: base.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ByAgent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d buckets, want one per agent", len(got))
	}

	// Ordered by spend, and a call is not a run: three calls over two runs is
	// what the agent did, and reporting either number as the other would make a
	// chatty agent look busy or a busy one look cheap.
	if got[0].Agent != "triage" || got[0].Micros != 900 ||
		got[0].Calls != 3 || got[0].Runs != 2 {
		t.Fatalf("first bucket = %+v, want triage folded across its runs", got[0])
	}
	// The unpriced call crosses into this cut too. Its tokens are here and its
	// money is not, which is the only honest way to show a rate nobody set.
	if got[0].Unpriced != 1 || got[0].InputTokens != 5_140 {
		t.Fatalf("first bucket = %+v, want the unpriced call counted with its tokens", got[0])
	}
	// This cut groups by agent alone, so it names no model. Carrying one would
	// be picking an arbitrary row out of a bucket that folded several.
	if got[0].Provider != "" || got[0].Model != "" {
		t.Fatalf("first bucket = %+v, want no model on the agent cut", got[0])
	}
	if got[1].Agent != "billing" || got[1].Micros != 400 || got[1].Runs != 1 {
		t.Fatalf("second bucket = %+v, want billing behind it", got[1])
	}
}

func TestSpend_aggregatesOnlyVisibleScopes(t *testing.T) {
	pool := spendPoolFor(t)
	store := ledger.NewPostgres(pool)
	spend := finops.NewSpend(pool)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	seedSpendCursor(t, pool, base)

	priced := domain.ModelPriceUse{Status: domain.ModelPriceConfigured}
	appendScopedRun(t, store, "run-platform", "triage",
		domain.Scope{Company: "acme", Area: "platform"}, base.Add(time.Second),
		call{
			cost:    domain.Cost{Micros: 700},
			planned: domain.PlannedPayload{Provider: "anthropic", Model: "opus", Price: &priced},
		})
	appendScopedRun(t, store, "run-private", "triage",
		domain.Scope{Company: "acme", Area: "finance"}, base.Add(2*time.Second),
		call{
			cost:    domain.Cost{Micros: 900},
			planned: domain.PlannedPayload{Provider: "anthropic", Model: "opus", Price: &priced},
		})
	appendScopedRun(t, store, "run-other-company", "triage",
		domain.Scope{Company: "globex", Area: "platform"}, base.Add(3*time.Second),
		call{
			cost:    domain.Cost{Micros: 1100},
			planned: domain.PlannedPayload{Provider: "anthropic", Model: "opus", Price: &priced},
		})

	if _, err := spend.Project(t.Context(), 100); err != nil {
		t.Fatalf("Project: %v", err)
	}

	got, err := spend.ByModel(t.Context(), domain.RunFilter{
		Since: base, Until: base.Add(time.Hour),
		Scopes: []domain.Scope{{Company: "acme", Area: "platform"}},
	})
	if err != nil {
		t.Fatalf("ByModel: %v", err)
	}
	if len(got) != 1 || got[0].Micros != 700 {
		t.Fatalf("scoped ByModel = %+v, want only acme/platform spend", got)
	}

	companyWide, err := spend.ByModel(t.Context(), domain.RunFilter{
		Since: base, Until: base.Add(time.Hour),
		Scopes: []domain.Scope{{Company: "acme"}},
	})
	if err != nil {
		t.Fatalf("ByModel company: %v", err)
	}
	if len(companyWide) != 1 || companyWide[0].Micros != 1600 {
		t.Fatalf("company ByModel = %+v, want both acme areas only", companyWide)
	}

	all, err := spend.ByModel(t.Context(), domain.RunFilter{
		Since: base, Until: base.Add(time.Hour),
		Scopes: []domain.Scope{{Company: domain.Installation}},
	})
	if err != nil {
		t.Fatalf("ByModel installation: %v", err)
	}
	if len(all) != 1 || all[0].Micros != 2700 {
		t.Fatalf("installation ByModel = %+v, want every company", all)
	}

	mixed, err := spend.ByModel(t.Context(), domain.RunFilter{
		Since: base, Until: base.Add(time.Hour),
		Scopes: []domain.Scope{
			{Company: "acme", Area: "platform"},
			{Company: domain.Installation},
		},
	})
	if err != nil {
		t.Fatalf("ByModel mixed installation: %v", err)
	}
	if len(mixed) != 1 || mixed[0].Micros != 2700 {
		t.Fatalf("mixed installation ByModel = %+v, want installation to dominate smaller scopes", mixed)
	}
}

func TestSpend_projectionStartSurvivesCursorMovement(t *testing.T) {
	pool := spendPoolFor(t)
	store := ledger.NewPostgres(pool)
	spend := finops.NewSpend(pool)
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	seedSpendCursor(t, pool, base)

	started, err := spend.ProjectedFrom(t.Context())
	if err != nil {
		t.Fatalf("ProjectedFrom: %v", err)
	}
	appendPlanned(t, store, "run-a", base.Add(time.Second),
		domain.Cost{Micros: 900},
		domain.PlannedPayload{Provider: "anthropic", Model: "opus"})
	if _, err := spend.Project(t.Context(), 100); err != nil {
		t.Fatalf("Project: %v", err)
	}
	after, err := spend.ProjectedFrom(t.Context())
	if err != nil {
		t.Fatalf("ProjectedFrom after: %v", err)
	}
	if !after.Equal(started) {
		t.Fatalf("projection start moved from %s to %s", started, after)
	}
}
