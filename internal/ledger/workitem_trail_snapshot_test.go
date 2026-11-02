package ledger_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

/*
The role summary is one snapshot (review round 2).

The summary bounds run_steps by the card's first and last opening moments.
Read in one statement and aggregated in another, under read committed, a run
that commits between the two — opened later than the bound the first read —
is counted with its cost by the second while its approvals fall outside the
stale bound: the role would count the run and lose what it asked. So the run
and its approvals are counted together, or neither is.

The interleaving is staged, not hoped for: the tracing pool commits a later
run, its claim and an approval through another connection just before the
aggregate is sent.
*/

// insertClaimedRun commits a real run of agent opened at, claiming card,
// with one approval asked — straight into the tables, as a concurrent
// worker's commit would land.
func insertClaimedRun(t *testing.T, pool *pgxpool.Pool, id string, agent domain.AgentID, at time.Time) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`insert into runs (run_id, company_id, area_id, agent_id, version_id, phase, last_seq,
		  started_at, updated_at, cost_micros)
		 values ($1, 'acme', 'factory', $2, 'v3', 'awaiting_approval', 3, $3, $3, 10)`,
		`insert into run_work_items (run_id, company_id, area_id, agent_id, ref, claimed_seq, claimed_at)
		 values ($1, 'acme', 'factory', $2, '` + string(card) + `', 2, $3)`,
		`insert into run_steps (run_id, seq, kind, company_id, area_id, agent_id, version_id,
		  at, opened_at, prev_hash, hash)
		 select $1, s.seq, s.kind, 'acme', 'factory', $2, 'v3', $3, $3,
		        case when s.seq = 1 then null else '\x01'::bytea end, '\x02'::bytea
		 from (values (1, 'run_started'), (2, 'approval_requested')) s(seq, kind)`,
	} {
		if _, err := pool.Exec(ctx, stmt, id, string(agent), at); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
}

func TestWorkItemTrail_aRunCommittedDuringTheSummary_isCountedWithItsApprovalsOrNotAtAll(t *testing.T) {
	s, pool, counter := countedLedger(t)
	first, second := trailMonths(t, pool)
	insertClaimedRun(t, pool, "run-early-1", "developer", first)
	insertClaimedRun(t, pool, "run-early-2", "developer", first.Add(time.Hour))

	var once sync.Once
	staged := false
	counter.mu.Lock()
	counter.before = func(sql string) {
		if strings.Contains(sql, "group by t.agent_id") {
			once.Do(func() {
				insertClaimedRun(t, pool, "run-late", "developer", second.Add(time.Hour))
				staged = true
			})
		}
	}
	counter.mu.Unlock()

	got, err := s.WorkItemTrail(context.Background(), domain.RunFilter{}, &domain.RunFilter{}, card, domain.WorkItemPage{})
	if err != nil {
		t.Fatalf("WorkItemTrail: %v", err)
	}
	if !staged || len(got.Roles) != 1 {
		t.Fatalf("staged %v, roles %+v; want the late commit staged and one role", staged, got.Roles)
	}
	requireTrailAgrees(t, got, 1, 10)
}

/*
The whole first page is one snapshot (review round 3): the timeline and the
summary describe the same runs. A run the summary counts appears on the
timeline or can be paged to, and one the timeline shows is counted — with
its approvals and its cost — so a reader never sees a role of three runs
beside a trail that ends at two.
*/
func TestWorkItemTrail_aSinglePage_agreesWithItsRoleSummary(t *testing.T) {
	run(t, "page and summary agree", func(t *testing.T, s Store) {
		for i, id := range []domain.RunID{"run-1", "run-2", "run-3"} {
			agent := domain.AgentID([]string{"triage", "developer", "triage"}[i])
			trailRunIn(t, s, id, agent, areaFactory, i, card)
			approvalsOn(t, s, id, agent, areaFactory, true)
			costed(t, s, id, agent, areaFactory, 10)
			endRun(t, s, id, agent, areaFactory)
		}

		got := trailOf(t, s, domain.RunFilter{}, &domain.RunFilter{}, domain.WorkItemPage{})
		requireTrailAgrees(t, got, 1, 10)
	})
}

// requireTrailAgrees holds a complete first page (no next) to its own
// summary: the roles count exactly the runs the timeline shows, and the
// approvals and cost that many runs carry — each run with approvalsPerRun
// approvals asked and costPerRun micros spent.
func requireTrailAgrees(t *testing.T, got domain.WorkItemTrail, approvalsPerRun int, costPerRun int64) {
	t.Helper()
	if got.Next != nil {
		t.Fatalf("next = %+v; this check is for a trail on one page", got.Next)
	}
	runs, asked, cost := 0, 0, int64(0)
	for _, r := range got.Roles {
		runs, asked = runs+r.Runs, asked+r.Approvals.Requested
		if r.CostMicros == nil {
			t.Fatalf("role %s has no cost; the test reads every scope's cost", r.AgentID)
		}
		cost += *r.CostMicros
	}
	shown := len(got.Runs)
	t.Logf("timeline %d runs; roles %d runs, %d approvals, %d micros", shown, runs, asked, cost)
	if runs != shown || asked != shown*approvalsPerRun || cost != int64(shown)*costPerRun {
		t.Fatalf("the timeline shows %d runs but the roles count %d runs, %d approvals, %d micros",
			shown, runs, asked, cost)
	}
}
