package ledger_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/ledger"
)

/*
0115 only adds (NT-018 phase 3): the turns, and a column on the proposals
that the previous binary neither reads nor writes. Nothing else that existed
at 0114 changes shape.
*/
func TestMigrate_upgradeTo0115_addsTheTurnsAndChangesNothingElse(t *testing.T) {
	pool := freshDatabase(t, "ledger/upgrade_0115")
	ctx := t.Context()
	if err := ledger.MigrateThrough(ctx, pool, "0114_agent_draft_proposals"); err != nil {
		t.Fatalf("migrate through 0114: %v", err)
	}
	before := schemaShape(ctx, t, pool)

	if err := ledger.MigrateThrough(ctx, pool, "0115_agent_draft_turns"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	after := schemaShape(ctx, t, pool)
	for _, line := range before {
		if !slices.Contains(after, line) {
			t.Errorf("0115 changed what was there: %s", line)
		}
	}
	added := 0
	for _, line := range after {
		if slices.Contains(before, line) {
			continue
		}
		added++
		own := strings.HasPrefix(line, "agent_draft_turns ") ||
			strings.HasPrefix(line, "agent_draft_proposals column turn_id ") ||
			strings.HasPrefix(line, "agent_draft_proposals constraint agent_draft_proposals_turn_id_fkey ")
		if !own {
			t.Errorf("0115 added outside the turns and the proposals' turn_id: %s", line)
		}
	}
	if added == 0 {
		t.Fatal("0115 added nothing")
	}
}
