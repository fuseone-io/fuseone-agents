package ledger_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/ledger"
)

/*
0128 widens the proposal's kinds by one, edit_block, and only widens: a row
the previous binary wrote still passes, the new kind is stored, and a kind
that is no operation is still refused. One check on the kind, not two: a
constraint dropped under the wrong name would leave the old check in place,
refusing the new kind while the new one admitted it.
*/
func TestMigrate_upgradeTo0128_admitsEditBlockAndKeepsEveryOtherKind(t *testing.T) {
	pool := freshDatabase(t, "ledger/upgrade_0128")
	ctx := t.Context()
	if err := ledger.MigrateThrough(ctx, pool, "0127_case_world_erasure"); err != nil {
		t.Fatalf("migrate through 0127: %v", err)
	}
	var thread string
	if err := pool.QueryRow(ctx, `insert into agent_draft_threads (draft_id, agent_id, author, created_at)
		values (gen_random_uuid(), 'triage', 'usr_ana', now()) returning thread_id`).Scan(&thread); err != nil {
		t.Fatalf("thread: %v", err)
	}
	if err := proposeKind(t, pool, thread, 1, "replace_block"); err != nil {
		t.Fatalf("a replace_block before the upgrade: %v", err)
	}
	if err := proposeKind(t, pool, thread, 2, "edit_block"); err == nil {
		t.Fatal("0127 already stores an edit_block; the fixture does not reach the upgrade")
	}

	if err := ledger.MigrateThrough(ctx, pool, "0128_agent_draft_proposal_edit_block"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if err := proposeKind(t, pool, thread, 2, "edit_block"); err != nil {
		t.Errorf("an edit_block after the upgrade: %v", err)
	}
	if err := proposeKind(t, pool, thread, 3, "grant_admin"); err == nil {
		t.Error("a kind that is no operation was stored")
	}
	var checks int
	if err := pool.QueryRow(ctx, `select count(*) from pg_constraint
		where conrelid = 'agent_draft_proposals'::regclass and contype = 'c'
		and pg_get_constraintdef(oid) like '%kind%'`).Scan(&checks); err != nil {
		t.Fatalf("count the checks: %v", err)
	}
	if checks != 1 {
		t.Errorf("%d checks on the proposal's kind, want one", checks)
	}
}

func proposeKind(t *testing.T, pool *pgxpool.Pool, thread string, seq int, kind string) error {
	t.Helper()
	_, err := pool.Exec(t.Context(), `insert into agent_draft_proposals
		(thread_id, seq, draft_id, agent_id, author, draft_revision, kind, digest, body_ref, valid, created_at)
		values ($1, $2, gen_random_uuid(), 'triage', 'usr_ana', '"r1"', $3, 'd', 'ref', true, now())`, thread, seq, kind)
	return err
}
