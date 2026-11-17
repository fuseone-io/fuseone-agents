package ledger_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/ledger"
)

/*
0114 only adds (NT-018 phase 3): the threads, the proposals and their
decisions. Nothing that existed at 0113 changes shape, so 0.118.0 keeps
running against the upgraded schema and a rollback to it leaves the
conversation's tables unread.
*/
func TestMigrate_upgradeTo0114_addsTheConversationTablesAndChangesNothingElse(t *testing.T) {
	pool := freshDatabase(t, "ledger/upgrade_0114")
	ctx := t.Context()
	if err := ledger.MigrateThrough(ctx, pool, "0113_agent_drafts"); err != nil {
		t.Fatalf("migrate through 0113: %v", err)
	}
	before := schemaShape(ctx, t, pool)

	if err := ledger.Migrate(ctx, pool); err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	after := schemaShape(ctx, t, pool)
	for _, line := range before {
		if !slices.Contains(after, line) {
			t.Errorf("0114 changed what was there: %s", line)
		}
	}
	added := 0
	for _, line := range after {
		if slices.Contains(before, line) {
			continue
		}
		added++
		own := false
		for _, table := range []string{"agent_draft_threads ", "agent_draft_proposals ", "agent_draft_proposal_decisions "} {
			own = own || strings.HasPrefix(line, table)
		}
		if !own {
			t.Errorf("0114 added outside its own tables: %s", line)
		}
	}
	if added == 0 {
		t.Fatal("0114 added nothing; the conversation's tables are missing")
	}
}
