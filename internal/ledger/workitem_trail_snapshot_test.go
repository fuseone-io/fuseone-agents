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

	got, err := s.WorkItemTrail(context.Background(), domain.RunFilter{}, nil, card, domain.WorkItemPage{})
	if err != nil {
		t.Fatalf("WorkItemTrail: %v", err)
	}
	if !staged || len(got.Roles) != 1 {
		t.Fatalf("staged %v, roles %+v; want the late commit staged and one role", staged, got.Roles)
	}
	dev := got.Roles[0]
	t.Logf("developer: %d runs, %d approvals asked", dev.Runs, dev.Approvals.Requested)
	if dev.Approvals.Requested != dev.Runs {
		t.Fatalf("developer counts %d runs but %d approvals: a run was counted without what it asked",
			dev.Runs, dev.Approvals.Requested)
	}
}
