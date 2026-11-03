package ledger_test

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The weekly measures stay bounded by the window, not by the ledger (#201).

Asserted on the production statement, explained against a ledger where the
window's runs are a handful among tens of thousands opened in other months
and other areas: the runs come by an index on runs, the approval steps are
read from the window's months alone — pruned when the statement is planned,
since the bounds are parameters — and a request is one statement however
many runs it counts.
*/

// otherMeasuredRuns is the rest of the history: runs in other months, and
// runs in the window's weeks but in another area.
const otherMeasuredRuns = 20_000

// measuresPlanWindow makes a year of partitions from four months ago and
// answers a two-week window two months into it, so the test does not depend
// on when it runs and there are months on both sides of the window.
func measuresPlanWindow(t *testing.T, pool *pgxpool.Pool) (domain.WorkItemWindow, time.Time) {
	t.Helper()
	now := time.Now().UTC()
	base := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -4, 0)
	if _, err := ledger.NewPartitions(pool, func() time.Time { return base }).Ensure(context.Background()); err != nil {
		t.Fatalf("partitions: %v", err)
	}
	w, err := domain.MeasureWindow(base.AddDate(0, 2, 14), 2)
	if err != nil {
		t.Fatalf("window: %v", err)
	}
	return w, base
}

// windowMonths is the partitions a window overlaps.
func windowMonths(w domain.WorkItemWindow) []string {
	var out []string
	for m := time.Date(w.From.Year(), w.From.Month(), 1, 0, 0, 0, 0, time.UTC); m.Before(w.To); m = m.AddDate(0, 1, 0) {
		out = append(out, partitionOf(m))
	}
	return out
}

// seedOtherMeasuredRuns is half the history in other months of the window's
// area, half in the window's weeks in another area.
func seedOtherMeasuredRuns(t *testing.T, pool *pgxpool.Pool, w domain.WorkItemWindow, base time.Time) {
	t.Helper()
	ctx := context.Background()
	for _, seed := range []struct {
		prefix, area string
		at           time.Time
	}{
		{"run-old-", "factory", base.Add(time.Hour)},
		{"run-cx-", "cx", w.From.Add(time.Hour)},
	} {
		if _, err := pool.Exec(ctx, `insert into runs
			(run_id, company_id, area_id, agent_id, version_id, phase, last_seq, cost_micros,
			 started_at, updated_at, ended_at)
			select $1 || g, 'acme', $2, 'developer', 'v3', 'finished', 2, 10,
			       $3::timestamptz + (g || ' seconds')::interval, now(), now()
			from generate_series(1, $4) g`, seed.prefix, seed.area, seed.at, otherMeasuredRuns/2); err != nil {
			t.Fatalf("seed %s: %v", seed.prefix, err)
		}
	}
	// Their claims and inheritances are history too: the links are reached
	// from the window's runs, never read whole.
	for _, stmt := range []string{
		`insert into run_work_items (run_id, company_id, area_id, agent_id, ref, claimed_seq, claimed_at)
		 select run_id, company_id, area_id, agent_id, 'github:o/r#' || substr(run_id, 9), 2, started_at
		 from runs where run_id like 'run-old-%' or run_id like 'run-cx-%'`,
		`insert into run_work_item_origins (run_id, company_id, area_id, agent_id, ref, source, started_at)
		 select run_id, company_id, area_id, agent_id, 'github:o/r#1', 'github_delivery', started_at
		 from runs where run_id like 'run-old-%' or run_id like 'run-cx-%'`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("seed the history's links: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `analyze runs; analyze run_work_items; analyze run_work_item_origins`); err != nil {
		t.Fatalf("analyze: %v", err)
	}
}

// historyTables are the tables that grow with the whole ledger: the
// measures reach them from the window's runs, never by reading them whole.
var historyTables = []string{"runs", "run_work_items", "run_work_item_origins"}

func TestWorkItemMeasures_plan_runsByIndexAndStepsPrunedToTheWindowsMonths(t *testing.T) {
	s, pool := feedPool(t)
	w, base := measuresPlanWindow(t, pool)
	for i := range 3 {
		id := domain.RunID(fmt.Sprintf("run-window-%d", i))
		openAt(t, s, id, "developer", areaFactory, w.From.Add(time.Duration(i)*24*time.Hour))
		refuse(t, s, id, "developer", areaFactory, propose(t, s, id, "developer", areaFactory, "github.issue_write", "d1"))
	}
	seedOtherMeasuredRuns(t, pool, w, base)

	sql, args := ledger.WorkItemMeasuresSQL(domain.RunFilter{Scopes: []domain.Scope{areaFactory}}, w)
	plan := explainJSON(t, pool, sql, args...)

	var partitions []string
	byIndex := false
	for _, n := range planNodes(plan, nil) {
		if strings.HasPrefix(n.Relation, "run_steps_") && !slices.Contains(partitions, n.Relation) {
			partitions = append(partitions, n.Relation)
		}
		if slices.Contains(historyTables, n.Relation) && n.NodeType == "Seq Scan" {
			t.Errorf("the measures scan %s whole", n.Relation)
		}
		byIndex = byIndex || strings.HasPrefix(n.Index, "runs_")
		if n.Relation != "" || n.Index != "" {
			t.Logf("planned: %s %s %s", n.NodeType, n.Relation, n.Index)
		}
	}
	slices.Sort(partitions)
	t.Logf("run_steps partitions planned: %v; runs by index: %v", partitions, byIndex)
	if want := windowMonths(w); !slices.Equal(partitions, want) {
		t.Errorf("the plan reads %v, want only the window's months %v, pruned when planned", partitions, want)
	}
	if !byIndex {
		t.Error("runs are not reached by an index")
	}
}

func TestWorkItemMeasures_aRequestIsOneStatement(t *testing.T) {
	s, _, counter := countedLedger(t)
	w := measuresWindow(t)
	for i := range 10 {
		id, agent := domain.RunID(fmt.Sprintf("run-%02d", i)), domain.AgentID(fmt.Sprintf("role-%02d", i%3))
		trailRunIn(t, s, id, agent, areaFactory, i, card)
		approvalsOn(t, s, id, agent, areaFactory, false, true)
		endRun(t, s, id, agent, areaFactory)
	}

	counter.take()
	rows, err := s.WorkItemMeasures(context.Background(), domain.RunFilter{}, everyCost, w)
	if err != nil || len(rows) != 3 {
		t.Fatalf("WorkItemMeasures = %+v, %v; want three roles", rows, err)
	}
	if asked := counter.take(); len(asked) != 1 {
		t.Errorf("the measures ran %d statements, want 1: %q", len(asked), asked)
	}
}

// The week is UTC's, not the session's: with the connection in São Paulo,
// Sunday's last microsecond and Monday's first in UTC still land where the
// memory store puts them.
func TestWorkItemMeasures_aSessionOutsideUTC_agreesWithMemoryAtTheSundayBoundary(t *testing.T) {
	dsn := testdb.DSN(t)
	if dsn == "" {
		requireDatabase(t, dsn)
		t.Skip("TEST_DATABASE_URL is unset")
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	stores := map[string]Store{
		"memory":   ledger.NewMemory(),
		"postgres": openPostgres(t, dsn+sep+"timezone=America/Sao_Paulo"),
	}
	got := map[string][]domain.WorkItemWeek{}
	for name, s := range stores {
		openAt(t, s, "run-sunday", "developer", areaFactory, trailWeek.Add(-time.Microsecond))
		// 21:00 on Sunday in São Paulo: Monday 00:00 UTC.
		openAt(t, s, "run-monday", "developer", areaFactory, trailWeek.In(time.FixedZone("BRT", -3*60*60)))
		// 23:00 on Sunday in São Paulo: Monday 02:00 UTC, the same week.
		openAt(t, s, "run-late", "developer", areaFactory, trailWeek.Add(2*time.Hour))
		got[name] = measuresOf(t, s, domain.RunFilter{}, everyCost)
	}

	if !reflect.DeepEqual(got["memory"], got["postgres"]) {
		t.Fatalf("stores disagree:\n memory   %+v\n postgres %+v", got["memory"], got["postgres"])
	}
	if rows := got["postgres"]; len(rows) != 2 || !rows[0].Week.Equal(trailWeek.AddDate(0, 0, -7)) ||
		rows[0].Runs != 1 || !rows[1].Week.Equal(trailWeek) || rows[1].Runs != 2 {
		t.Fatalf("rows = %+v; want Sunday's run alone in the week before", rows)
	}
}
