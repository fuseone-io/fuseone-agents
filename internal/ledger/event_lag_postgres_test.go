package ledger_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

// holdXmin opens a transaction that takes an id — the oldest in flight, as a
// long report or a stuck migration would be — and returns when it began by
// its own clock. It is rolled back when the test ends.
func holdXmin(t *testing.T, pool *pgxpool.Pool) (pgx.Tx, time.Time) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	var began time.Time
	var id string
	if err := tx.QueryRow(ctx, `select now(), pg_current_xact_id()::text`).Scan(&began, &id); err != nil {
		t.Fatalf("take an id: %v", err)
	}
	return tx, began
}

/*
A run that committed past an open transaction is counted as held by xmin,
not as the dispatcher's backlog: sweeping more often would not reach it. The
holder's start is the transaction's own, so its age grows with the clock, and
once it ends the same run is the dispatcher's to read.
*/
func TestEventDispatchBacklog_anOpenTransaction_holdsTheRunAndAges(t *testing.T) {
	s, pool := feedPool(t)
	ctx := context.Background()
	tx, began := holdXmin(t, pool)
	finishRunIn(t, s, "run-later")

	held := backlogOf(t, s)
	if held.HeldByXmin != 1 || held.Ready != 0 || !held.Held() {
		t.Fatalf("backlog = %+v; want the run held by xmin, none ready", held)
	}
	if !held.Xact.Open || !held.Xact.StartedAt.Equal(began) {
		t.Fatalf("holder = %+v; want the open transaction, begun %v", held.Xact, began)
	}
	now := began.Add(10 * time.Minute)
	before, _ := held.XactAge(now)
	after, known := held.XactAge(now.Add(time.Minute))
	if !known || after-before != time.Minute || before != 10*time.Minute {
		t.Fatalf("age %v then %v (known %v); want it to grow with the clock", before, after, known)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	released := backlogOf(t, s)
	if released.HeldByXmin != 0 || released.Ready != 1 {
		t.Fatalf("after the transaction ended: %+v; want the run ready", released)
	}
}

/*
A run inserted straight into 'finished' carries no ended_at — the column is
nullable and 0092 stamps such a row with a position all the same. It waits
like any other, so its wait must count: the lag falls back to the instant the
row was last written rather than reading a positive backlog as caught up.
*/
func TestEventDispatchBacklog_aFinishedRunWithNoEndedAt_stillHasAWait(t *testing.T) {
	s, pool := feedPool(t)
	ctx := context.Background()
	written := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `insert into runs
		(run_id, company_id, area_id, agent_id, version_id, phase, last_seq, started_at, updated_at)
		values ('run-inserted', 'acme', 'cx', 'triage', 'v3', 'finished', 2, $1::timestamptz - interval '1 minute', $1)`,
		written); err != nil {
		t.Fatalf("terminal insert: %v", err)
	}

	got := backlogOf(t, s)
	if got.Ready+got.HeldByXmin != 1 {
		t.Fatalf("backlog = %+v; want the inserted run waiting", got)
	}
	if !got.OldestFinishedAt.Equal(written) {
		t.Fatalf("oldest finished at %v, want %v: the row's own last write", got.OldestFinishedAt, written)
	}
	if status := got.Status(written.Add(time.Hour)); status != domain.EventDispatchDegraded {
		t.Fatalf("status an hour later = %q; a run waiting an hour is not healthy", status)
	}
}

// unprivileged opens the ledger as a role that holds no monitoring grant and
// may read only what the feed reads — what an installation that connects as
// a plain application role looks like. The role is dropped when the test ends.
func unprivileged(t *testing.T, admin *pgxpool.Pool) *ledger.Postgres {
	t.Helper()
	ctx := context.Background()
	role := fmt.Sprintf("lag_probe_%d", time.Now().UnixNano())
	for _, stmt := range []string{
		`create role ` + role + ` nologin`,
		`grant usage on schema public to ` + role,
		`grant select on runs, event_dispatch_cursor to ` + role,
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	cfg := admin.Config().Copy()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, `set role `+role)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect as %s: %v", role, err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), `drop owned by `+role)
		_, _ = admin.Exec(context.Background(), `drop role `+role)
	})
	return ledger.NewPostgres(pool)
}

/*
A role without pg_read_all_stats sees that another session holds a
transaction id, not when it began. The backlog still reads — the holder is
open and its age unknown — rather than failing the health page or reporting
no holder at all.
*/
func TestEventDispatchBacklog_aRoleThatCannotSeeOtherSessions_reportsTheAgeUnknown(t *testing.T) {
	s, pool := feedPool(t)
	plain := unprivileged(t, pool)
	holdXmin(t, pool)
	finishRunIn(t, s, "run-later")

	got, err := plain.EventDispatchBacklog(context.Background())
	if err != nil {
		t.Fatalf("EventDispatchBacklog as a plain role: %v", err)
	}
	if got.HeldByXmin != 1 || !got.Xact.Open || !got.Xact.StartedAt.IsZero() {
		t.Fatalf("backlog = %+v; want the run held and the holder open with no start", got)
	}
	if _, known := got.XactAge(time.Now()); known {
		t.Fatal("the holder's age is reported known; the role cannot see it")
	}
}

/*
An installation that revoked pg_stat_activity from its application role
still reads the backlog: runs held past xmin prove a holder is open, though
nothing says since when.
*/
func TestEventDispatchBacklog_withoutPgStatActivity_stillReadsTheBacklog(t *testing.T) {
	s, pool := feedPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `revoke select on pg_catalog.pg_stat_activity from public`); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `grant select on pg_catalog.pg_stat_activity to public`)
	})
	plain := unprivileged(t, pool)
	holdXmin(t, pool)
	finishRunIn(t, s, "run-later")

	got, err := plain.EventDispatchBacklog(ctx)
	if err != nil {
		t.Fatalf("EventDispatchBacklog without pg_stat_activity: %v", err)
	}
	if got.HeldByXmin != 1 || !got.Xact.Open || !got.Xact.StartedAt.IsZero() {
		t.Fatalf("backlog = %+v; want the run held and a holder of unknown age", got)
	}
}
