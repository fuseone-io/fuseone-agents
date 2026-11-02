package ledger_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
One card with a long life costs a page what a page holds (#204 review).

A card a stuck role keeps retrying gathers thousands of runs. The first page
must still read its limit of runs, and the per-role summary over all of them
must be an aggregate in the database — a bounded number of statements, no
argument that grows with the card's history, and run_steps read only in the
months the card's runs opened in.
*/

// bigCard is the size of the history: 5,000 claims on one reference.
const bigCard = 5_000

// seedBigCard writes bigCard finished runs of three roles claiming card,
// spread over three months, each with one approval asked and decided
// (approved on even runs); and a hundred runs of another card in a fourth
// month, whose steps a pruned read never touches.
func seedBigCard(t *testing.T, pool *pgxpool.Pool, months []time.Time) {
	t.Helper()
	ctx := context.Background()
	seed := func(prefix, ref string, n int, month func(int) time.Time) {
		t.Helper()
		starts := make([]time.Time, n)
		for i := range n {
			starts[i] = month(i).Add(time.Duration(i) * time.Second)
		}
		batch := []string{
			`insert into runs (run_id, company_id, area_id, agent_id, version_id, phase, last_seq,
			  started_at, updated_at, ended_at, cost_micros)
			 select $1 || g, 'acme', 'factory', 'role-' || (g % 3), 'v3', 'finished', 3,
			        ($3::timestamptz[])[g], ($3::timestamptz[])[g], ($3::timestamptz[])[g], 10
			 from generate_series(1, $2) g where $4::text <> ''`,
			`insert into run_work_items (run_id, company_id, area_id, agent_id, ref, claimed_seq, claimed_at)
			 select $1 || g, 'acme', 'factory', 'role-' || (g % 3), $4, 2, ($3::timestamptz[])[g]
			 from generate_series(1, $2) g`,
			`insert into run_steps (run_id, seq, kind, company_id, area_id, agent_id, version_id,
			  payload, at, opened_at, prev_hash, hash)
			 select $1 || g, s.seq, s.kind, 'acme', 'factory', 'role-' || (g % 3), 'v3',
			        case when s.kind = 'approval_decided'
			             then jsonb_build_object('approved', g % 2 = 0, 'by', 'ana') else '{}'::jsonb end,
			        ($3::timestamptz[])[g], ($3::timestamptz[])[g],
			        case when s.seq = 1 then null else '\x01'::bytea end, '\x02'::bytea
			 from generate_series(1, $2) g,
			      (values (1, 'run_started'), (2, 'approval_requested'), (3, 'approval_decided')) s(seq, kind)
			 where $4::text <> ''`,
		}
		for _, stmt := range batch {
			if _, err := pool.Exec(ctx, stmt, prefix, n, starts, ref); err != nil {
				t.Fatalf("seed %s: %v", prefix, err)
			}
		}
	}
	seed("run-big-", string(card), bigCard, func(i int) time.Time { return months[i%3] })
	seed("run-other-", string(otherCard), 100, func(int) time.Time { return months[3] })
	if _, err := pool.Exec(ctx, `analyze runs; analyze run_work_items; analyze run_steps`); err != nil {
		t.Fatalf("analyze: %v", err)
	}
}

// fourMonths are four consecutive months the ledger has partitions for.
func fourMonths(t *testing.T, pool *pgxpool.Pool) []time.Time {
	t.Helper()
	first, _ := trailMonths(t, pool)
	return []time.Time{first, first.AddDate(0, 1, 0), first.AddDate(0, 2, 0), first.AddDate(0, 3, 0)}
}

func TestWorkItemTrail_aCardWithALongHistory_readsABoundedFirstPage(t *testing.T) {
	s, pool, counter := countedLedger(t)
	months := fourMonths(t, pool)
	seedBigCard(t, pool, months)
	factory := domain.RunFilter{Scopes: []domain.Scope{areaFactory}}

	counter.take()
	got, err := s.WorkItemTrail(context.Background(), factory, &factory, card, domain.WorkItemPage{Limit: 50})
	if err != nil {
		t.Fatalf("WorkItemTrail: %v", err)
	}
	widest, most := counter.widestList(), counter.mostRows() // before take, which resets them
	asked := len(counter.take())

	if len(got.Runs) != 50 || got.Next == nil {
		t.Fatalf("first page = %d runs, next %v; want the limit of 50 and more to come", len(got.Runs), got.Next)
	}
	runs, approved, refused, cost := 0, 0, 0, int64(0)
	for _, r := range got.Roles {
		runs, approved, refused = runs+r.Runs, approved+r.Approvals.Approved, refused+r.Approvals.Refused
		if r.CostMicros != nil {
			cost += *r.CostMicros
		}
	}
	if len(got.Roles) != 3 || runs != bigCard || approved != bigCard/2 || refused != bigCard/2 || cost != 10*bigCard {
		t.Fatalf("roles = %+v; want three roles over all %d runs, half approved, all costed", got.Roles, bigCard)
	}
	t.Logf("first page: %d statements, widest list argument %d, most rows from one statement %d", asked, widest, most)
	if asked > 3 || widest > 0 || most > 51 {
		t.Fatalf("first page ran %d statements, a list of %d, up to %d rows; want at most 3, no list, the page plus one",
			asked, widest, most)
	}
}

func TestWorkItemTrail_plan_theRoleSummaryIsAnAggregatePrunedToTheCardsMonths(t *testing.T) {
	_, pool, _ := countedLedger(t)
	months := fourMonths(t, pool)
	seedBigCard(t, pool, months)
	factory := domain.RunFilter{Scopes: []domain.Scope{areaFactory}}

	lo, hi := months[0], months[2].Add(time.Duration(bigCard)*time.Second)
	sql, args := ledger.WorkItemRolesSQL(factory, card, lo, hi)
	plan := explainJSON(t, pool, sql, args...)

	// Here the card is nearly the whole of run_work_items, so reading it whole
	// is the right plan; the ref index is asserted where the card is a sliver
	// of the table, in TestWorkItemTrail_plan_runsByTheRefIndex... This test
	// holds the summary to an aggregate over the card's months only.
	aggregate := false
	for _, n := range planNodes(plan, nil) {
		aggregate = aggregate || n.NodeType == "Aggregate"
	}
	var steps []string
	for _, r := range scannedRelations(plan) {
		if strings.HasPrefix(r, "run_steps") {
			steps = append(steps, r)
		}
	}
	want := []string{partitionOf(months[0]), partitionOf(months[1]), partitionOf(months[2])}
	t.Logf("role summary: aggregate %v, run_steps read %v", aggregate, steps)
	if !aggregate || !slices.Equal(steps, want) {
		t.Fatalf("role summary plan: aggregate %v, reads %v; want an aggregate over only %v", aggregate, steps, want)
	}
}
