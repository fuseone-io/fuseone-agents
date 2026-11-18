package ledger_test

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

func TestMigrate_upgradeTo0117SeedsManualLegacyProvenance(t *testing.T) {
	pool := freshDatabase(t, "ledger/upgrade_0117")
	ctx := t.Context()
	if err := ledger.MigrateThrough(ctx, pool, "0116_agent_draft_proposal_reasons"); err != nil {
		t.Fatalf("migrate through 0116: %v", err)
	}
	legacy := draft.New("legacy-provenance", "Legacy", domain.Scope{Company: "acme", Area: "cx"})
	legacy.Spec.Tools = []domain.ToolID{"crm.lookup"}
	legacy.Blocks = []blocks.Block{{ID: "b1", Text: "Typed by hand."}}
	stored, err := draft.Encode(legacy)
	if err != nil {
		t.Fatal(err)
	}
	var draftID string
	if err := pool.QueryRow(ctx, `select gen_random_uuid()::text`).Scan(&draftID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		insert into agent_drafts
		  (agent_id, draft_id, company_id, area_id, definition, blocks, revision,
		   created_by, created_at, updated_by, updated_at)
		values ('legacy-provenance', $1::uuid, 'acme', 'cx', $2, $3, 'legacy-revision',
		        'usr_ana', now(), 'usr_ana', now())`, draftID, stored.Definition, stored.Blocks); err != nil {
		t.Fatalf("insert legacy draft: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into agent_draft_trail
		  (draft_id, seq, agent_id, action, origin, revision, by_user, at)
		values ($1::uuid, 1, 'legacy-provenance', 'create', 'control',
		        'legacy-revision', 'usr_ana', now())`, draftID); err != nil {
		t.Fatalf("insert legacy trail: %v", err)
	}

	if err := ledger.MigrateThrough(ctx, pool, "0117_agent_draft_provenance"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	var sources []string
	var revision string
	var seq int
	if err := pool.QueryRow(ctx, `
		select (select array_agg(part order by part) from agent_draft_sources where draft_id = $1::uuid),
		       sources_revision, sources_seq
		from agent_drafts where draft_id = $1::uuid`, draftID).Scan(&sources, &revision, &seq); err != nil {
		t.Fatal(err)
	}
	want := []string{"block/b1", "name", "scope", "standingApprovals", "tools/crm.lookup"}
	if !slices.Equal(sources, want) || revision != "legacy-revision" || seq != 1 {
		t.Fatalf("seeded sources=%v stamp=(%q,%d), want %v and legacy-revision/1",
			sources, revision, seq, want)
	}
}
