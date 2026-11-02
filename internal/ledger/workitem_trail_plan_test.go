package ledger_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The trail's reads stay bounded by the card, not by the ledger (#204).

Asserted on the production statements, explained against a ledger where the
card is a handful of runs among tens of thousands claimed for other cards,
across months: the runs come by run_work_items_ref_idx, and the approval
counts are one statement over run_steps that touches only the partitions of
the months the card's runs opened in. And the trail is at most two
statements a page, however many runs it holds — never one per run.
*/

// otherCards is the rest of the factory's history: claims for other cards.
const otherCards = 20_000

// trailMonths opens the card's runs in two months the ledger has partitions
// for, made here so the test does not depend on when it runs.
func trailMonths(t *testing.T, pool *pgxpool.Pool) (time.Time, time.Time) {
	t.Helper()
	now := time.Now().UTC()
	first := time.Date(now.Year(), now.Month(), 1, 9, 0, 0, 0, time.UTC)
	if _, err := ledger.NewPartitions(pool, func() time.Time { return first }).Ensure(context.Background()); err != nil {
		t.Fatalf("partitions: %v", err)
	}
	return first, first.AddDate(0, 1, 0)
}

// seedOtherCards claims otherCards real runs, each for a card of its own.
func seedOtherCards(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`insert into runs (run_id, company_id, area_id, agent_id, version_id, phase, last_seq,
		  started_at, updated_at, ended_at)
		 select 'run-other-' || g, 'acme', 'factory', 'triage', 'v3', 'finished', 2,
		        now() - (g || ' minutes')::interval, now(), now()
		 from generate_series(1, $1) g`,
		`insert into run_work_items (run_id, company_id, area_id, agent_id, ref, claimed_seq, claimed_at)
		 select 'run-other-' || g, 'acme', 'factory', 'triage',
		        'github:fuseone-io/fuseone-agents#' || (1000 + g), 2, now() - (g || ' minutes')::interval
		 from generate_series(1, $1) g`,
	} {
		if _, err := pool.Exec(ctx, stmt, otherCards); err != nil {
			t.Fatalf("seed other cards: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `analyze runs; analyze run_work_items`); err != nil {
		t.Fatalf("analyze: %v", err)
	}
}

// explainJSON explains a production statement with its arguments.
func explainJSON(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) planNode {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(context.Background(), `explain (format json) `+sql, args...).Scan(&raw); err != nil {
		t.Fatalf("explain: %v", err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("decode plan: %v (%s)", err, raw)
	}
	return plans[0].Plan
}

func planNodes(n planNode, into []planNode) []planNode {
	into = append(into, n)
	for _, child := range n.Plans {
		into = planNodes(child, into)
	}
	return into
}

func TestWorkItemTrail_plan_runsByTheRefIndexAndStepsPrunedToTheirMonths(t *testing.T) {
	s, pool := feedPool(t)
	first, second := trailMonths(t, pool)
	seedOtherCards(t, pool)
	for i, at := range []time.Time{first, second, second.Add(time.Hour)} {
		id := domain.RunID(fmt.Sprintf("run-card-%d", i))
		agent := domain.AgentID(fmt.Sprintf("role-%d", i))
		started := step(id, domain.StepRunStarted)
		started.AgentID, started.Scope, started.At = agent, areaFactory, at
		mustAppend(t, s, started)
		claimed := claimStep(t, id, agent, card)
		claimed.Scope, claimed.At = areaFactory, at
		mustAppend(t, s, claimed)
	}

	filter := domain.RunFilter{Scopes: []domain.Scope{areaFactory}}
	requireRefIndex(t, pool, "the first page", filter, nil, 0)
	at := &domain.WorkItemCursor{ClaimedAt: first, RunID: "run-card-0"}
	requireRefIndex(t, pool, "a later page", filter, at, domain.DefaultWorkItemPage+1)

	// The bounds the store reads before the summary: the card's first and
	// last opening moment.
	sql, args := ledger.WorkItemRolesSQL(filter, card, first, second.Add(time.Hour))
	plan := explainJSON(t, pool, sql, args...)
	var read []string
	for _, r := range scannedRelations(plan) {
		if strings.HasPrefix(r, "run_steps") {
			read = append(read, r)
		}
	}
	refIndex := false
	for _, n := range planNodes(plan, nil) {
		refIndex = refIndex || n.Index == "run_work_items_ref_idx"
	}
	want := []string{partitionOf(first), partitionOf(second)}
	t.Logf("the role summary reads %v, ref index %v", read, refIndex)
	if !slices.Equal(read, want) || !refIndex {
		t.Errorf("the role summary reads %v (ref index %v); want only the card's months %v, by the ref index",
			read, refIndex, want)
	}
}

// requireRefIndex explains the runs statement and asserts the card's runs
// are found by run_work_items_ref_idx, never by reading the table whole.
func requireRefIndex(
	t *testing.T, pool *pgxpool.Pool, what string, filter domain.RunFilter, after *domain.WorkItemCursor, limit int,
) {
	t.Helper()
	sql, args := ledger.WorkItemRunsSQL(filter, card, after, limit)
	usesRefIndex := false
	for _, n := range planNodes(explainJSON(t, pool, sql, args...), nil) {
		if n.Relation == "run_work_items" && n.NodeType == "Seq Scan" {
			t.Errorf("%s: the runs statement scans run_work_items whole", what)
		}
		usesRefIndex = usesRefIndex || n.Index == "run_work_items_ref_idx"
	}
	t.Logf("%s: run_work_items_ref_idx used = %v", what, usesRefIndex)
	if !usesRefIndex {
		t.Errorf("%s: the runs statement does not use run_work_items_ref_idx", what)
	}
}

func partitionOf(month time.Time) string { return "run_steps_" + month.Format("2006_01") }

// scannedRelations is every relation a plan reads, sorted and distinct.
func scannedRelations(root planNode) []string {
	var out []string
	for _, n := range planNodes(root, nil) {
		if n.Relation != "" && !slices.Contains(out, n.Relation) {
			out = append(out, n.Relation)
		}
	}
	slices.Sort(out)
	return out
}

/*
The append never projects a simulation's claim, so run_work_items should hold
none. The trail does not lean on that alone: a simulated run that has a row
all the same — written by an older release, or by hand — stays off the trail
because the runs projection says it is simulated.
*/
func TestWorkItemTrail_aSimulatedRunWithAProjectedClaim_staysOffTheTrail(t *testing.T) {
	s, pool := feedPool(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`insert into runs (run_id, company_id, area_id, agent_id, version_id, phase, last_seq,
		  started_at, updated_at, simulated)
		 values ('run-sim', 'acme', 'factory', 'developer', 'v3', 'running', 2, now(), now(), true)`,
		`insert into run_work_items (run_id, company_id, area_id, agent_id, ref, claimed_seq, claimed_at)
		 values ('run-sim', 'acme', 'factory', 'developer', '` + string(card) + `', 2, now())`,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("plant a simulated claim: %v", err)
		}
	}

	got, err := s.WorkItemTrail(ctx, domain.RunFilter{}, nil, card, domain.WorkItemPage{})
	if err != nil || len(got.Runs) != 0 || len(got.Roles) != 0 {
		t.Fatalf("trail = %+v, %v; want the simulation absent", got, err)
	}
}

// queryCounter counts the statements a pool is asked to run, and the widest
// list any of them was handed as an argument.
type queryCounter struct {
	mu     sync.Mutex
	asked  []string
	widest int
	// most is the most rows any one statement returned.
	most int64
}

func (q *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.asked = append(q.asked, strings.Join(strings.Fields(d.SQL), " "))
	for _, arg := range d.Args {
		if v := reflect.ValueOf(arg); v.Kind() == reflect.Slice && v.Type().Elem().Kind() != reflect.Uint8 {
			q.widest = max(q.widest, v.Len())
		}
	}
	return ctx
}

func (q *queryCounter) TraceQueryEnd(_ context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.most = max(q.most, d.CommandTag.RowsAffected())
}

func (q *queryCounter) take() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := q.asked
	q.asked, q.widest, q.most = nil, 0, 0
	return out
}

func (q *queryCounter) widestList() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.widest
}

func (q *queryCounter) mostRows() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.most
}

// countedLedger is the Postgres ledger over a pool that reports every
// statement it runs.
func countedLedger(t *testing.T) (*ledger.Postgres, *pgxpool.Pool, *queryCounter) {
	t.Helper()
	_, plain := feedPool(t) // migrated and emptied
	cfg, err := pgxpool.ParseConfig(testdb.DSN(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	counter := &queryCounter{}
	cfg.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return ledger.NewPostgres(pool), plain, counter
}

// Ten runs, each with approvals: the first page is three statements (the
// page, the card's bounds, the role summary) and a later one is one. A read
// per run would be twelve.
func TestWorkItemTrail_statementsPerPage_doNotGrowWithTheRuns(t *testing.T) {
	s, _, counter := countedLedger(t)
	for i := range 10 {
		id := domain.RunID(fmt.Sprintf("run-%02d", i))
		agent := domain.AgentID(fmt.Sprintf("role-%02d", i))
		trailRunIn(t, s, id, agent, areaFactory, i, card)
		approvalsOn(t, s, id, agent, areaFactory, true)
	}

	counter.take()
	first, err := s.WorkItemTrail(context.Background(), domain.RunFilter{}, nil, card, domain.WorkItemPage{Limit: 4})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if asked := counter.take(); len(asked) != 3 {
		t.Errorf("the first page ran %d statements, want 3 (page, bounds, roles): %q", len(asked), asked)
	}
	if _, err := s.WorkItemTrail(context.Background(), domain.RunFilter{}, nil, card,
		domain.WorkItemPage{Limit: 4, After: first.Next}); err != nil {
		t.Fatalf("second page: %v", err)
	}
	if asked := counter.take(); len(asked) != 1 {
		t.Errorf("a later page ran %d statements, want 1: %q", len(asked), asked)
	}
}
