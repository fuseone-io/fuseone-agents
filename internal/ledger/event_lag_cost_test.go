package ledger_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

// backlogRuns is a backlog fifty times the count cap: what a day of a stuck
// transaction, or a day of simulations, leaves behind on a busy installation.
const backlogRuns = 50_000

// feedPage is the dispatcher's page (trigger.Page).
const feedPage = 30

// feedIndex is the index the feed and the backlog read: real finished runs
// only (0094).
const feedIndex = "runs_feed_idx"

// planNode is the part of EXPLAIN (ANALYZE, FORMAT JSON) the bound is read from.
type planNode struct {
	NodeType       string     `json:"Node Type"`
	Relation       string     `json:"Relation Name"`
	Index          string     `json:"Index Name"`
	ActualRows     float64    `json:"Actual Rows"`
	ActualLoops    float64    `json:"Actual Loops"`
	RemovedByFiltr float64    `json:"Rows Removed by Filter"`
	Plans          []planNode `json:"Plans"`
}

// runsScans collects every node of the plan that reads the runs table.
func runsScans(n planNode, into []planNode) []planNode {
	if n.Relation == "runs" {
		into = append(into, n)
	}
	for _, child := range n.Plans {
		into = runsScans(child, into)
	}
	return into
}

// seedRuns inserts n finished runs in one transaction, simulated or real.
func seedRuns(t *testing.T, pool *pgxpool.Pool, prefix string, n int, simulated bool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `insert into runs
		(run_id, company_id, area_id, agent_id, version_id, phase, last_seq,
		 started_at, updated_at, ended_at, simulated)
		select $1 || lpad(g::text, 6, '0'), 'acme', 'cx', 'triage', 'v3', 'finished', 2,
		       now(), now(), now(), $3
		from generate_series(1, $2) g`, prefix, n, simulated); err != nil {
		t.Fatalf("seed %s: %v", prefix, err)
	}
}

// historyRuns is the real history an installation carries from before the
// feed existed: finished runs with no position. Every other index on runs
// holds them; the feed's does not. Without them the planner can find another
// index as small as the feed's and prefer it, which no installation that has
// been running for a while would see.
const historyRuns = 20_000

func seedHistory(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	seedRuns(t, pool, "run-history-", historyRuns, false)
	if _, err := pool.Exec(context.Background(),
		`update runs set finished_xid = null where run_id like 'run-history-%'`); err != nil {
		t.Fatalf("unposition the history: %v", err)
	}
}

func analyzeRuns(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `analyze runs`); err != nil {
		t.Fatalf("analyze: %v", err)
	}
}

// requireBoundedPlan explains a production statement and asserts every read
// of runs is a range on the feed's index examining at most bound rows.
func requireBoundedPlan(t *testing.T, pool *pgxpool.Pool, what, sql string, bound int, args ...any) {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(context.Background(), `explain (analyze, format json) `+sql, args...).Scan(&raw); err != nil {
		t.Fatalf("explain %s: %v", what, err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("decode the %s plan: %v (%s)", what, err, raw)
	}
	scans := runsScans(plans[0].Plan, nil)
	if len(scans) == 0 {
		t.Fatalf("no read of runs in the %s plan: %s", what, raw)
	}
	for _, scan := range scans {
		examined := scan.ActualRows*scan.ActualLoops + scan.RemovedByFiltr
		t.Logf("%s: %s using %s: %.0f rows examined", what, scan.NodeType, scan.Index, examined)
		if scan.Index != feedIndex || examined > float64(bound) {
			t.Errorf("%s: %s on %q examined %.0f rows; want a range on %s of at most %d",
				what, scan.NodeType, scan.Index, examined, feedIndex, bound)
		}
	}
}

/*
The backlog read and the dispatcher's page stay bounded however large the
range past the cursor grows (#228). The backlog is read every minute on every
worker and on every health page view while xmin is stuck — exactly when that
range grows without limit. And simulations finish too: they are stamped with
a position but never offered, so the cursor never moves past them, and an
index that held them made every read walk all of them to reach the next real
run, for ever. So every read of runs is a range on the index of real
finished runs, and none touches more rows than its limit: the count cap plus
one for the backlog, the page for the feed. Asserted on the rows the plan
examined, not on wall time.
*/
func TestEventDispatchBacklog_aBacklogFarPastTheCap_readsABoundedRange(t *testing.T) {
	cases := []struct {
		name        string
		held        bool
		simulations bool
	}{
		{"ready", false, false},
		{"held by xmin", true, false},
		{"ready behind simulations", false, true},
		{"held behind simulations", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, pool := feedPool(t)
			seedHistory(t, pool)
			if c.held {
				holdXmin(t, pool)
			}
			real := backlogRuns
			if c.simulations {
				seedRuns(t, pool, "run-sim-", backlogRuns, true)
				real = 2 * domain.EventDispatchCountCap
			}
			seedRuns(t, pool, "run-real-", real, false)
			analyzeRuns(t, pool)

			got := backlogOf(t, s)
			// Held when this test holds xmin. Without its hold the runs are
			// ready — unless a transaction elsewhere on the cluster holds
			// them, which is why the ready cases read whichever is larger.
			count, capped := max(got.Ready, got.HeldByXmin), got.ReadyCapped || got.HeldCapped
			if c.held {
				// Each count says whether it was cut: here only the held one.
				count, capped = got.HeldByXmin, got.HeldCapped && !got.ReadyCapped
			}
			if !capped || count != domain.EventDispatchCountCap {
				t.Fatalf("backlog = %+v; want the count stopped at the cap of %d", got, domain.EventDispatchCountCap)
			}
			requireBoundedPlan(t, pool, "backlog", ledger.EventDispatchBacklogSQL,
				domain.EventDispatchCountCap+1, "0", "", domain.EventDispatchCountCap+1)
			requireBoundedPlan(t, pool, "feed page", ledger.FinishedSinceSQL, feedPage, "0", "", feedPage)
		})
	}
}
