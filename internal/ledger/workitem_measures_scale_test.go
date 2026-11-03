package ledger_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
What follows a refusal is read in one pass, however many refusals the window
holds (#201, review round 1).

Thousands of runs inside the window, each asking, being refused, and then —
by thirds — asking the same tool with the same arguments, with others, or
not again. Analysed, the statement never runs a subquery per row over the
window's approval steps — no SubPlan reads run_steps or a CTE built from
them — and the rows every scan examines, summed over its loops, stay a
small multiple of what the window holds. Linear in the window, not in
refusals times steps.
*/

const scaleRefusals = 3_000

// seedRefusals writes scaleRefusals runs straight into the projection and
// the steps, inside the window's first week: run_started, a request of
// github.issue_write with digest a, its refusal, and then by g % 3 the same
// request again (repeated), one with digest b (revised), or nothing.
func seedRefusals(t *testing.T, pool *pgxpool.Pool, w domain.WorkItemWindow) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`insert into runs (run_id, company_id, area_id, agent_id, version_id, phase, last_seq,
		  started_at, updated_at, cost_micros)
		 select 'run-refused-' || g, 'acme', 'factory', 'developer', 'v3', 'running', 4,
		        $1::timestamptz + (g || ' seconds')::interval, now(), 10
		 from generate_series(1, $2) g`,
		`insert into run_steps (run_id, seq, kind, company_id, area_id, agent_id, version_id,
		  payload, at, opened_at, prev_hash, hash)
		 select 'run-refused-' || g, s.seq, s.kind, 'acme', 'factory', 'developer', 'v3', s.payload,
		        $1::timestamptz + (g || ' seconds')::interval, $1::timestamptz + (g || ' seconds')::interval,
		        case when s.seq = 1 then null else '\x01'::bytea end, '\x02'::bytea
		 from generate_series(1, $2) g
		 cross join lateral (values
		     (1, 'run_started', '{}'::jsonb),
		     (2, 'approval_requested', '{"tool":"github.issue_write","args_digest":"a"}'::jsonb),
		     (3, 'approval_decided', '{"approved":false,"by":"ana","at_seq":2}'::jsonb),
		     (4, 'approval_requested', case when g % 3 = 0
		         then '{"tool":"github.issue_write","args_digest":"a"}'::jsonb
		         else '{"tool":"github.issue_write","args_digest":"b"}'::jsonb end)
		 ) s(seq, kind, payload)
		 where s.seq < 4 or g % 3 <> 2`,
	} {
		if _, err := pool.Exec(ctx, stmt, w.From, scaleRefusals); err != nil {
			t.Fatalf("seed refusals: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `analyze runs; analyze run_steps`); err != nil {
		t.Fatalf("analyze: %v", err)
	}
}

func TestWorkItemMeasures_thousandsOfRefusals_areReadInOnePassNotOnePerRefusal(t *testing.T) {
	s, pool := feedPool(t)
	w, base := measuresPlanWindow(t, pool)
	seedRefusals(t, pool, w)
	seedOtherMeasuredRuns(t, pool, w, base)

	// Bounded, so a quadratic statement fails here rather than hanging the
	// suite: the linear one answers in well under a second.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	rows, err := s.WorkItemMeasures(ctx, domain.RunFilter{}, everyCost, w)
	if err != nil {
		t.Fatalf("WorkItemMeasures: %v", err)
	}
	p := weekRow(t, rows, w.From, "developer").Proposals
	if third := scaleRefusals / 3; p.Refused != scaleRefusals || p.RepeatedAfterRefusal != third ||
		p.RevisedAfterRefusal != third {
		t.Fatalf("proposals = %+v; want %d refused, %d repeated, %d revised", p, scaleRefusals, third, third)
	}

	sql, args := ledger.WorkItemMeasuresSQL(domain.RunFilter{Scopes: []domain.Scope{areaFactory}}, w)
	plan := explainAnalyzeJSON(t, pool, sql, args...)
	var examined float64
	for _, n := range planNodes(plan, nil) {
		if n.ParentRelationship == "SubPlan" && readsSteps(n) {
			t.Errorf("a subquery runs once per row over the approval steps: %s %s%s", n.NodeType, n.Relation, n.CTEName)
		}
		if n.Relation != "" || n.CTEName != "" {
			examined += n.ActualLoops * (n.ActualRows + n.RemovedByFiltr)
		}
	}
	// What the window holds: each refused run's opening, request and
	// refusal, two thirds of them a follow-up, and the other area's runs.
	held := float64(3*scaleRefusals + 2*scaleRefusals/3 + otherMeasuredRuns/2)
	t.Logf("the window holds %.0f rows; every scan, over its loops, examined %.0f (%.1fx)", held, examined, examined/held)
	if examined > 10*held {
		t.Errorf("the scans examined %.0f rows for a window of %.0f: more than linear", examined, held)
	}
}

// readsSteps reports whether a node or anything under it reads run_steps or
// a CTE built from it.
func readsSteps(n planNode) bool {
	if strings.HasPrefix(n.Relation, "run_steps") || (n.CTEName != "" && n.CTEName != "x" && n.CTEName != "r") {
		return true
	}
	for _, c := range n.Plans {
		if readsSteps(c) {
			return true
		}
	}
	return false
}
