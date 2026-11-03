package ledger_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Inherited runs keep the trail bounded by the card (#204, slice 3).

The trail now reads two relations of attributions — claims and inheritances —
and judges each inheritance against its run's claim. Over a ledger where both
tables hold tens of thousands of rows for other cards, every statement must
still reach the card's rows by each table's ref index and the run's claim or
inheritance by its primary key, never read either table whole, and the role
summary must still read only the partitions of the months the card's runs
opened in.
*/

// seedOtherOrigins inherits otherCards real runs, each a card of its own.
func seedOtherOrigins(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`insert into runs (run_id, company_id, area_id, agent_id, version_id, phase, last_seq,
		  started_at, updated_at, ended_at)
		 select 'run-inherit-' || g, 'acme', 'factory', 'reviewer', 'v3', 'finished', 2,
		        now() - (g || ' minutes')::interval, now(), now()
		 from generate_series(1, $1) g`,
		`insert into run_work_item_origins (run_id, company_id, area_id, agent_id, ref, source, from_run, started_at)
		 select 'run-inherit-' || g, 'acme', 'factory', 'reviewer',
		        'github:fuseone-io/fuseone-agents#' || (1000 + g), 'event', 'run-other-' || g,
		        now() - (g || ' minutes')::interval
		 from generate_series(1, $1) g`,
	} {
		if _, err := pool.Exec(ctx, stmt, otherCards); err != nil {
			t.Fatalf("seed other inheritances: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `analyze runs; analyze run_work_item_origins`); err != nil {
		t.Fatalf("analyze: %v", err)
	}
}

func TestWorkItemTrail_plan_inheritedRunsByTheirRefIndexAndStepsPrunedToTheirMonths(t *testing.T) {
	s, pool := feedPool(t)
	first, second := trailMonths(t, pool)
	seedOtherCards(t, pool)
	seedOtherOrigins(t, pool)
	for i, at := range []time.Time{first, second} {
		id := domain.RunID(fmt.Sprintf("run-card-%d", i))
		agent := domain.AgentID(fmt.Sprintf("role-%d", i))
		started := step(id, domain.StepRunStarted)
		started.AgentID, started.Scope, started.At = agent, areaFactory, at
		mustAppend(t, s, started)
		claimed := claimStep(t, id, agent, card)
		claimed.Scope, claimed.At = areaFactory, at
		mustAppend(t, s, claimed)
	}
	inherited := inheritedStart(t, "run-inherits", "reviewer", areaFactory, 0, fromEvent(card, "run-card-0"))
	inherited.At = second.Add(time.Hour)
	mustAppend(t, s, inherited)
	elsewhere := claimStep(t, "run-inherits", "reviewer", otherCard)
	elsewhere.Scope, elsewhere.At = areaFactory, inherited.At
	mustAppend(t, s, elsewhere)

	filter := domain.RunFilter{Scopes: []domain.Scope{areaFactory}}
	firstPage, args := ledger.WorkItemRunsSQL(filter, card, nil, 0)
	requireBothRefIndexes(t, "the first page", explainJSON(t, pool, firstPage, args...))
	at := &domain.WorkItemCursor{ClaimedAt: first, RunID: "run-card-0"}
	laterPage, args := ledger.WorkItemRunsSQL(filter, card, at, domain.DefaultWorkItemPage+1)
	requireBothRefIndexes(t, "a later page", explainJSON(t, pool, laterPage, args...))

	sql, args := ledger.WorkItemRolesSQL(filter, card)
	plan := explainInSnapshot(t, pool, sql, args...)
	requireBothRefIndexes(t, "the role summary", plan)
	read, pruned := runStepsPartitions(plan)
	want := []string{partitionOf(first), partitionOf(second)}
	t.Logf("the role summary reads %v; pruned while running: %s", read, pruned)
	if !slices.Equal(read, want) {
		t.Errorf("the role summary reads %v; want only the card's months %v", read, want)
	}

	got, err := s.WorkItemTrail(context.Background(), filter, nil, card, domain.WorkItemPage{})
	if err != nil || len(got.Runs) != 3 || got.Runs[2].ClaimedRef != otherCard {
		t.Fatalf("trail = %+v, %v; want both claims and the inherited run, flagged", got.Runs, err)
	}
}

// requireBothRefIndexes asserts a plan reaches claims and inheritances by
// their ref indexes and reads neither table whole.
func requireBothRefIndexes(t *testing.T, what string, plan planNode) {
	t.Helper()
	used := map[string]bool{}
	for _, n := range planNodes(plan, nil) {
		if (n.Relation == "run_work_items" || n.Relation == "run_work_item_origins") && n.NodeType == "Seq Scan" {
			t.Errorf("%s: scans %s whole", what, n.Relation)
		}
		used[n.Index] = true
	}
	t.Logf("%s: claims by ref index %v, inheritances by ref index %v",
		what, used["run_work_items_ref_idx"], used["run_work_item_origins_ref_idx"])
	for _, index := range []string{"run_work_items_ref_idx", "run_work_item_origins_ref_idx"} {
		if !used[index] {
			t.Errorf("%s: does not use %s", what, index)
		}
	}
}

// Inherited runs add no statement: the first page is still two, a later one.
func TestWorkItemTrail_withInheritedRuns_statementsPerPageDoNotGrow(t *testing.T) {
	s, _, counter := countedLedger(t)
	for i := range 6 {
		id := domain.RunID(fmt.Sprintf("run-%02d", i))
		agent := domain.AgentID(fmt.Sprintf("role-%02d", i))
		if i%2 == 0 {
			trailRunIn(t, s, id, agent, areaFactory, i, card)
		} else {
			inheritIn(t, s, id, agent, areaFactory, i, delivered(card))
		}
		approvalsOn(t, s, id, agent, areaFactory, true)
	}

	counter.take()
	first, err := s.WorkItemTrail(context.Background(), domain.RunFilter{}, nil, card, domain.WorkItemPage{Limit: 4})
	if err != nil || len(first.Runs) != 4 {
		t.Fatalf("first page: %+v, %v", first.Runs, err)
	}
	if asked := counter.take(); len(asked) != 2 {
		t.Errorf("the first page ran %d statements, want 2: %q", len(asked), asked)
	}
	if role := roleOf(t, first.Roles, "role-01"); role.Runs != 1 || role.Approvals.Approved != 1 {
		t.Errorf("an inherited run's role = %+v, want it counted with its approval", role)
	}
	if _, err := s.WorkItemTrail(context.Background(), domain.RunFilter{}, nil, card,
		domain.WorkItemPage{Limit: 4, After: first.Next}); err != nil {
		t.Fatalf("second page: %v", err)
	}
	if asked := counter.take(); len(asked) != 1 {
		t.Errorf("a later page ran %d statements, want 1: %q", len(asked), asked)
	}
}

// The projected row is the run's own, written once, and never changed.
func TestWorkItemOrigin_projectedRow_isTheRunsAndAppendOnly(t *testing.T) {
	s, pool := feedPool(t)
	inheritIn(t, s, "run-x", "reviewer", areaFactory, 0, fromEvent(card, "run-dev"))
	inheritIn(t, s, "run-hook", "triage", areaFactory, 1, delivered(card))
	started := inheritedStart(t, "run-sim", "triage", areaFactory, 2, delivered(card))
	started.Payload = mustJSON(t, domain.RunStartedPayload{Simulated: true, Simulation: "s", WorkItem: delivered(card)})
	mustAppend(t, s, started)

	ctx := context.Background()
	var company, area, agent, ref, source, from string
	if err := pool.QueryRow(ctx, `select company_id, area_id, agent_id, ref, source, coalesce(from_run, '')
		from run_work_item_origins where run_id = 'run-x'`).Scan(&company, &area, &agent, &ref, &source, &from); err != nil {
		t.Fatalf("read the projection: %v", err)
	}
	if company != "acme" || area != "factory" || agent != "reviewer" || ref != string(card) ||
		source != "event" || from != "run-dev" {
		t.Fatalf("row = %s %s %s %s %s %s", company, area, agent, ref, source, from)
	}
	var claims, sims int
	if err := pool.QueryRow(ctx, `select (select count(*) from run_work_items),
		(select count(*) from run_work_item_origins where run_id = 'run-sim')`).Scan(&claims, &sims); err != nil {
		t.Fatalf("count: %v", err)
	}
	if claims != 0 || sims != 0 {
		t.Fatalf("run_work_items rows = %d, simulated origins = %d; want none of either", claims, sims)
	}
	if _, err := pool.Exec(ctx, `update run_work_item_origins set ref = 'x' where run_id = 'run-x'`); err == nil {
		t.Fatal("run_work_item_origins accepted an update")
	}
	if _, err := pool.Exec(ctx, `delete from run_work_item_origins where run_id = 'run-x'`); err == nil {
		t.Fatal("run_work_item_origins accepted a delete")
	}
}
