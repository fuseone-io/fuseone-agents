package ledger_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

// --- Postgres only: the alias projection -------------------------------------

func aliasRows(t *testing.T, pool *pgxpool.Pool, ref domain.WorkItemRef) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(),
		`select count(*) from work_item_aliases where ref = $1`, string(ref)).Scan(&n); err != nil {
		t.Fatalf("count aliases: %v", err)
	}
	return n
}

// The row is the linking run's: its agent, company and area, from the runs
// projection, and the link's own seq. The same link again projects nothing.
func TestWorkItemLink_projectsOneRowWithTheRunsIdentity(t *testing.T) {
	store, pool := openWorkItemPostgres(t)
	openLinked(t, store, "run-a", "developer", linkAlias, card)
	proof := returnPullRequest(t, store, "run-a", "developer", linkAlias)
	if err := linkWith(t, store, "run-a", "developer", linked(linkAlias, card, proof)); err != nil {
		t.Fatalf("the same link again: %v", err)
	}

	var root, company, area, agent, runID string
	var seq int64
	if err := pool.QueryRow(t.Context(), `
		select root_ref, company_id, area_id, agent_id, run_id, linked_seq
		from work_item_aliases where ref = $1`, string(linkAlias),
	).Scan(&root, &company, &area, &agent, &runID, &seq); err != nil {
		t.Fatalf("read the alias row: %v", err)
	}
	if root != string(card) || company != "acme" || area != "cx" || agent != "developer" || runID != "run-a" || seq != 5 {
		t.Fatalf("alias row = %s %s/%s %s %s @%d; want the card, the run's identity, seq 5",
			root, company, area, agent, runID, seq)
	}
	if n := aliasRows(t, pool, linkAlias); n != 1 {
		t.Fatalf("alias rows after the same link twice = %d, want 1", n)
	}
}

func TestWorkItemLink_simulatedRun_projectsNothing(t *testing.T) {
	store, pool := openWorkItemPostgres(t)
	openSimulatedRun(t, store, "run-sim", "developer")
	if err := claim(t, store, "run-sim", "developer", card); err != nil {
		t.Fatalf("simulated claim: %v", err)
	}
	proof := returnPullRequest(t, store, "run-sim", "developer", linkAlias)
	if err := linkWith(t, store, "run-sim", "developer", linked(linkAlias, card, proof)); err != nil {
		t.Fatalf("simulated link: %v", err)
	}
	if n := aliasRows(t, pool, linkAlias); n != 0 {
		t.Fatalf("alias rows for a simulated link = %d, want none", n)
	}
}

func TestWorkItemAliases_updateOrDelete_rejected(t *testing.T) {
	store, pool := openWorkItemPostgres(t)
	openLinked(t, store, "run-a", "developer", linkAlias, card)
	ctx := t.Context()

	if _, err := pool.Exec(ctx, `update work_item_aliases set root_ref = 'x' where ref = $1`, string(linkAlias)); err == nil {
		t.Fatal("work_item_aliases accepted an update; an alias never changes root")
	}
	if _, err := pool.Exec(ctx, `delete from work_item_aliases where ref = $1`, string(linkAlias)); err == nil {
		t.Fatal("work_item_aliases accepted a delete; a link is never removed")
	}
	if n := aliasRows(t, pool, linkAlias); n != 1 {
		t.Fatalf("alias rows after the refused writes = %d, want 1", n)
	}
}

/*
Two runs link one pull request to two cards at once: exactly one wins.

Staged like the claim's race (#206). The winner's link is sealed inside a
transaction held open; the loser's Append is let go and observed waiting on an
alias lock in pg_stat_activity; only then does the winner commit. A loser that
read the company's links before taking the lock — or in the statement that
takes it — would find no root and project a second one.
*/
func TestWorkItemLink_concurrentLinksOfOneAlias_exactlyOneWins(t *testing.T) {
	store, pool := openWorkItemPostgres(t)
	ctx := t.Context()
	openClaimed(t, store, "run-winner", "developer", card)
	winProof := returnPullRequest(t, store, "run-winner", "developer", linkAlias)
	openClaimed(t, store, "run-loser", "developer", otherCard)
	loseProof := returnPullRequest(t, store, "run-loser", "developer", linkAlias)

	winner, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin the winner: %v", err)
	}
	defer func() { _ = winner.Rollback(context.Background()) }()
	if _, err := ledger.AppendInTx(ctx, winner,
		linkStepOf(t, "run-winner", "developer", linked(linkAlias, card, winProof))); err != nil {
		t.Fatalf("hold the winner's link: %v", err)
	}

	lost := make(chan error, 1)
	go func() {
		_, err := store.Append(ctx, linkStepOf(t, "run-loser", "developer", linked(linkAlias, otherCard, loseProof)))
		lost <- err
	}()

	waitForAliasLockWait(t, pool)
	if err := winner.Commit(ctx); err != nil {
		t.Fatalf("commit the winner: %v", err)
	}
	if err := <-lost; !errors.Is(err, domain.ErrWorkItemLinkedElsewhere) {
		t.Fatalf("the loser's link = %v, want ErrWorkItemLinkedElsewhere", err)
	}
	if n := aliasRows(t, pool, linkAlias); n != 1 {
		t.Fatalf("alias rows = %d, want exactly one", n)
	}
}

// The nesting race: a card gains an alias while it is itself being linked as
// one. Both take the lock on the shared reference, so the second sees the
// first.
func TestWorkItemLink_concurrentRootBecomingAnAlias_refused(t *testing.T) {
	store, pool := openWorkItemPostgres(t)
	ctx := t.Context()
	openClaimed(t, store, "run-winner", "developer", card)
	winProof := returnPullRequest(t, store, "run-winner", "developer", linkAlias)
	openClaimed(t, store, "run-loser", "qa", thirdCard)
	loseProof := returnPullRequest(t, store, "run-loser", "qa", card)

	winner, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin the winner: %v", err)
	}
	defer func() { _ = winner.Rollback(context.Background()) }()
	if _, err := ledger.AppendInTx(ctx, winner,
		linkStepOf(t, "run-winner", "developer", linked(linkAlias, card, winProof))); err != nil {
		t.Fatalf("hold the winner's link: %v", err)
	}
	lost := make(chan error, 1)
	go func() {
		_, err := store.Append(ctx, linkStepOf(t, "run-loser", "qa", linked(card, thirdCard, loseProof)))
		lost <- err
	}()
	waitForAliasLockWait(t, pool)
	if err := winner.Commit(ctx); err != nil {
		t.Fatalf("commit the winner: %v", err)
	}
	if err := <-lost; !errors.Is(err, domain.ErrWorkItemLinkNested) {
		t.Fatalf("linking a card that just gained an alias = %v, want ErrWorkItemLinkNested", err)
	}
}

func waitForAliasLockWait(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := pool.QueryRow(t.Context(), `
			select count(*) from pg_stat_activity
			where wait_event_type = 'Lock' and wait_event = 'advisory'
			  and query like '%work_item_alias:%'
			  and pid <> pg_backend_pid()`).Scan(&waiting); err != nil {
			t.Fatalf("read lock waits: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the loser never waited on an alias lock; the race was not staged")
}
