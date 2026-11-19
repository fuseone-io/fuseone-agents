package trigger_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/trigger"
)

/*
Reconciling in the caller's transaction (NT-018 phase 2, release 2c).

An activation moves an agent's schedules and webhooks in the transaction that makes its
version current, under the agent's row, so a reconciliation can never land after a later
activation's. SyncIn writes only through the transaction it is given: undone with it,
nothing remains.
*/

func countFor(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`select count(*) from `+table+` where agent_id = 'triage'`).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestSyncIn_writesThroughTheCallersTransaction_andIsUndoneWithIt(t *testing.T) {
	pool := testdb.Pool(t)
	if _, err := pool.Exec(t.Context(), `truncate trigger_schedules, webhook_triggers`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// Deferred too: a failure below must not leave the transaction open, or the pool's
	// close waits for it forever.
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := trigger.NewPostgresSchedules(pool).SyncIn(t.Context(), tx, "triage", []string{"*/15 * * * *"}, noon); err != nil {
		t.Fatalf("schedules SyncIn: %v", err)
	}
	if err := trigger.NewPostgresWebhooks(pool).SyncIn(t.Context(), tx, "triage", cx,
		[]domain.WebhookTrigger{{Path: "crm/ticket"}}); err != nil {
		t.Fatalf("webhooks SyncIn: %v", err)
	}
	var inside int
	if err := tx.QueryRow(t.Context(), `select count(*) from trigger_schedules where agent_id = 'triage'`).Scan(&inside); err != nil || inside != 1 {
		t.Fatalf("inside the transaction: %d schedules (%v), want the one synced", inside, err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}

	if s, w := countFor(t, pool, "trigger_schedules"), countFor(t, pool, "webhook_triggers"); s != 0 || w != 0 {
		t.Errorf("after the rollback: %d schedules, %d webhooks; want nothing", s, w)
	}
}
