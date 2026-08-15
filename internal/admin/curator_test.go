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
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
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
	return pool
}

// freshPool is openPool with the shared tables emptied.
//
// Separate because openPool used to do both, and a helper that wipes shared
// state every time it is called is a trap: asking for a second pool in the
// middle of a test deleted what the test had just recorded, twice, before
// anybody noticed the two were the same function.
func freshPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := openPool(t)
	// Every settings kind a test in this package writes. Naming them one by
	// one rather than emptying the table keeps the fixtures a `make dev` left
	// behind, and the list grows with the package: a kind left off leaks
	// between tests and fails whichever one happens to run second.
	if _, err := pool.Exec(context.Background(),
		`delete from settings where kind in ('tool_classification', 'stop');
		 delete from admin_events`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	return pool
}

var platform = domain.Scope{Company: "default", Area: "platform"}

func TestClassify_survivesTheProcessThatMadeIt(t *testing.T) {
	pool := freshPool(t)
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
	pool := freshPool(t)
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
	pool := freshPool(t)

	// EffectUnknown is the zero value that makes an unclassified tool fail
	// closed. Storing it deliberately would look like a decision.
	err := admin.NewCurator(pool).Classify(context.Background(), platform,
		domain.ToolClassification{Tool: "crm.note", Effect: domain.EffectUnknown, By: "usr_ana"})
	if err == nil {
		t.Fatal("Classify accepted an unclassified effect as a ruling")
	}
}

func TestClassify_twice_keepsTheLatestAndBothRecords(t *testing.T) {
	pool := freshPool(t)
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

func TestTools_carryTheUndoTheCuratorDeclared(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	curator := admin.NewCurator(pool)

	if err := curator.Publish(ctx, []domain.ToolEntry{
		{ID: "crm.note", Server: "crm", Description: "escreve nota"},
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := curator.Classify(ctx, platform, domain.ToolClassification{
		Tool: "crm.note", Effect: domain.EffectWrite, By: "usr_ana",
		CompensatedBy: "crm.note.delete",
	}); err != nil {
		t.Fatalf("Classify: %v", err)
	}

	// The published catalogue and the rulings are two records that meet here.
	// The serve process reads only this, so a field the join forgets is a
	// ruling that was made, stored, and is invisible to everyone.
	entries, err := curator.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	// Found rather than counted: these tests share a pool, and a count asserts
	// about every other test as well as this one.
	note := find(t, entries, "crm.note")
	if note.CompensatedBy != "crm.note.delete" {
		t.Errorf("CompensatedBy = %q, want the tool that undoes it", note.CompensatedBy)
	}
}

func find(t *testing.T, entries []domain.ToolEntry, id domain.ToolID) domain.ToolEntry {
	t.Helper()
	for _, e := range entries {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("%s is not in %+v", id, entries)
	return domain.ToolEntry{}
}

/*
A tool nobody ruled on reads as unclassified, on the screen too.

The runtime already refuses it: discovery files it unclassified and the Gate's
contract check blocks. This is the other half, and without it the two halves
disagree — the console, the interview and the flow check all read this, so an
author would design against a screen saying "read, allowed" for a call the
platform would stop. Being told at authoring time is the whole point of showing
an effect at all.
*/
func TestTools_withNoRuling_readAsUnclassifiedRatherThanRead(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	curator := admin.NewCurator(pool)

	if err := curator.Publish(ctx, []domain.ToolEntry{
		{ID: "github.delete_repository", Server: "github", Description: "remove um repositório"},
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	entries, err := curator.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if got := find(t, entries, "github.delete_repository"); got.Effect != domain.EffectUnknown {
		t.Errorf("Effect = %v, want unknown until somebody rules", got.Effect)
	}
}
