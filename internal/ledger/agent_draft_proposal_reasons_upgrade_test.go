package ledger_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/ledger"
)

/*
0116 only adds (NT-018 phase 3): a reference to each proposal's reason in the
content store, which the previous binary neither reads nor writes. Nothing
else that existed at 0115 changes shape.
*/
func TestMigrate_upgradeTo0116_addsTheReasonReferenceAndNothingElse(t *testing.T) {
	pool := freshDatabase(t, "ledger/upgrade_0116")
	ctx := t.Context()
	if err := ledger.MigrateThrough(ctx, pool, "0115_agent_draft_turns"); err != nil {
		t.Fatalf("migrate through 0115: %v", err)
	}
	before := schemaShape(ctx, t, pool)

	if err := ledger.MigrateThrough(ctx, pool, "0116_agent_draft_proposal_reasons"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}

	after := schemaShape(ctx, t, pool)
	for _, line := range before {
		if !slices.Contains(after, line) {
			t.Errorf("0116 changed what was there: %s", line)
		}
	}
	var added []string
	for _, line := range after {
		if !slices.Contains(before, line) {
			added = append(added, line)
		}
	}
	// The column, and on Postgres 18 its NOT NULL as a named constraint.
	if len(added) == 0 {
		t.Fatal("0116 added nothing")
	}
	for _, line := range added {
		own := strings.HasPrefix(line, "agent_draft_proposals column reason_ref ") ||
			strings.HasPrefix(line, "agent_draft_proposals constraint agent_draft_proposals_reason_ref_not_null ")
		if !own {
			t.Errorf("0116 added outside agent_draft_proposals.reason_ref: %s", line)
		}
	}
}
