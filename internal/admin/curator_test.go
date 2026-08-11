package admin_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		// Skipped rather than silently absent: the whole point of these
		// assertions is that a ruling survives the process that made it.
		t.Skip("TEST_DATABASE_URL is unset; skipping the Curator suite")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := ledger.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`delete from settings where kind = 'tool_classification'; delete from admin_events`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	return pool
}

var platform = domain.Scope{Company: "default", Area: "platform"}

func TestClassify_survivesTheProcessThatMadeIt(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	// The Curator's act is the single point where write access enters the
	// platform. Holding it in memory meant a worker restart silently demoted
	// every tool back to read-only.
	if err := admin.NewCurator(pool).Classify(ctx, platform, domain.ToolClassification{
		Tool: "crm.note", Effect: domain.EffectWrite, By: "usr_ana", Reason: "escreve nota interna",
	}); err != nil {
		t.Fatalf("Classify: %v", err)
	}

	// A different Curator over the same database is the next process.
	rulings, err := admin.NewCurator(pool).List(ctx, platform)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rulings) != 1 || rulings[0].Effect != domain.EffectWrite {
		t.Fatalf("List = %+v, want the write ruling", rulings)
	}
	if rulings[0].By != "usr_ana" {
		t.Errorf("By = %q, want the person who ruled", rulings[0].By)
	}
}

func TestClassify_recordsWhoRuledInTheSameTransaction(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()

	if err := admin.NewCurator(pool).Classify(ctx, platform, domain.ToolClassification{
		Tool: "crm.refund", Effect: domain.EffectFinancial, By: "usr_ana",
	}); err != nil {
		t.Fatalf("Classify: %v", err)
	}

	var action, target, by string
	if err := pool.QueryRow(ctx,
		`select action, target, principal_id from admin_events order by event_id desc limit 1`,
	).Scan(&action, &target, &by); err != nil {
		t.Fatalf("read event: %v", err)
	}

	// A ruling that took effect with no record of who made it is precisely
	// what the administrative trail exists to make impossible.
	if action != "tool.classified" || target != "crm.refund" || by != "usr_ana" {
		t.Errorf("event = %s/%s by %s, want tool.classified/crm.refund by usr_ana", action, target, by)
	}
}

func TestClassify_unknownEffect_isRefusedRatherThanStored(t *testing.T) {
	pool := openPool(t)

	// EffectUnknown is the zero value that makes an unclassified tool fail
	// closed. Storing it deliberately would look like a decision.
	err := admin.NewCurator(pool).Classify(context.Background(), platform,
		domain.ToolClassification{Tool: "crm.note", Effect: domain.EffectUnknown, By: "usr_ana"})
	if err == nil {
		t.Fatal("Classify accepted an unclassified effect as a ruling")
	}
}

func TestClassify_twice_keepsTheLatestAndBothRecords(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	curator := admin.NewCurator(pool)

	for _, effect := range []domain.Effect{domain.EffectWrite, domain.EffectRead} {
		if err := curator.Classify(ctx, platform, domain.ToolClassification{
			Tool: "crm.note", Effect: effect, By: "usr_ana",
		}); err != nil {
			t.Fatalf("Classify(%s): %v", effect, err)
		}
	}

	rulings, _ := curator.List(ctx, platform)
	if len(rulings) != 1 || rulings[0].Effect != domain.EffectRead {
		t.Errorf("List = %+v, want the ruling narrowed to read", rulings)
	}

	// Corrections are new records, never amendments: demoting a tool is
	// exactly the change an auditor will ask about later.
	var events int
	_ = pool.QueryRow(ctx, `select count(*) from admin_events where target = 'crm.note'`).Scan(&events)
	if events != 2 {
		t.Errorf("admin_events = %d, want both rulings on record", events)
	}
}
