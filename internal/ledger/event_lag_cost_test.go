package ledger_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

// backlogRuns is a backlog fifty times the count cap: what a day of a stuck
// transaction leaves behind on a busy installation.
const backlogRuns = 50_000

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

/*
The backlog read stays bounded however large the backlog grows (#228). It runs
every minute on every worker and on every health page view while xmin is
stuck — exactly when the range past the cursor grows without limit. So every
read of runs is an index range from the cursor, and none touches more rows
than the count cap and the one it needs to know it was reached. Asserted on
the rows the plan examined, not on wall time.
*/
func TestEventDispatchBacklog_aBacklogFarPastTheCap_readsABoundedRange(t *testing.T) {
	s, pool := feedPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `insert into runs
		(run_id, company_id, area_id, agent_id, version_id, phase, last_seq, started_at, updated_at, ended_at)
		select 'run-' || lpad(g::text, 6, '0'), 'acme', 'cx', 'triage', 'v3', 'finished', 2, now(), now(), now()
		from generate_series(1, $1) g`, backlogRuns); err != nil {
		t.Fatalf("seed the backlog: %v", err)
	}
	if _, err := pool.Exec(ctx, `analyze runs`); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	got := backlogOf(t, s)
	if !got.Capped || max(got.Ready, got.HeldByXmin) != domain.EventDispatchCountCap {
		t.Fatalf("backlog = %+v; want a count stopped at the cap of %d", got, domain.EventDispatchCountCap)
	}

	var raw []byte
	if err := pool.QueryRow(ctx, `explain (analyze, format json) `+ledger.EventDispatchBacklogSQL,
		"0", "", domain.EventDispatchCountCap+1).Scan(&raw); err != nil {
		t.Fatalf("explain: %v", err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("decode the plan: %v (%s)", err, raw)
	}
	scans := runsScans(plans[0].Plan, nil)
	if len(scans) == 0 {
		t.Fatalf("no read of runs in the plan: %s", raw)
	}
	for _, scan := range scans {
		examined := scan.ActualRows*scan.ActualLoops + scan.RemovedByFiltr
		if scan.Index != "runs_finished_xid_idx" || examined > domain.EventDispatchCountCap+1 {
			t.Errorf("%s on %q examined %.0f rows; want an index range on runs_finished_xid_idx of at most %d",
				scan.NodeType, scan.Index, examined, domain.EventDispatchCountCap+1)
		}
	}
}
