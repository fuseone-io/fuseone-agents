package ledger_test

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
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
	draftID := insertLegacyDraft(t, pool, legacy)

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

func TestMigrate_upgradeTo0117LegacyNamesMatchEveryCurrentSourcePart(t *testing.T) {
	pool := freshDatabase(t, "ledger/upgrade_0117_part_parity")
	if err := ledger.MigrateThrough(t.Context(), pool, "0116_agent_draft_proposal_reasons"); err != nil {
		t.Fatalf("migrate through 0116: %v", err)
	}
	legacy := fullyPopulatedLegacyDraft()
	draftID := insertLegacyDraft(t, pool, legacy)
	if err := ledger.MigrateThrough(t.Context(), pool, "0117_agent_draft_provenance"); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	var got []string
	if err := pool.QueryRow(t.Context(), `
		select array_agg(part order by part) from agent_draft_sources
		where draft_id = $1::uuid`, draftID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	want := sourcePartNames(legacy)
	if len(want) != 16 {
		t.Fatalf("complete fixture has %d source parts, want 16: %v", len(want), want)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("migration source parts = %v, runtime source parts = %v", got, want)
	}
}

func fullyPopulatedLegacyDraft() draft.Draft {
	d := draft.New("legacy-all-parts", "Legacy", domain.Scope{Company: "acme", Area: "cx"})
	s := &d.Spec
	s.Provider, s.Model, s.Effort = "openai", "gpt-test", "high"
	s.Fallback = domain.ModelPair{Provider: "anthropic", Model: "claude-test"}
	s.CachePrefixTTL, s.ReplyLocale = "1h", "pt-BR"
	s.Tools = []domain.ToolID{"crm.lookup", "crm.reply"}
	s.Budget = domain.Budget{Micros: 100, Tokens: 200, ToolCalls: 3, Steps: 4, WallClockMS: 500}
	s.Triggers = []spec.Trigger{{Type: "cron", Schedule: "0 * * * *"}}
	s.Emits = spec.Emits{{Event: "ticket.triaged"}}
	s.Steps = []spec.Step{{Name: "Classify", Reaches: []domain.ToolID{"crm.lookup"}}}
	s.MemoryLearning = domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview}
	s.Approvals = domain.ApprovalPolicy{Direct: true, Notify: []domain.UserID{"usr_ana"}}
	d.Blocks = []blocks.Block{{ID: "b1", Text: "Typed by hand."}}
	return d
}

func sourcePartNames(d draft.Draft) []string {
	parts := draft.SourceParts(d)
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func insertLegacyDraft(t *testing.T, pool *pgxpool.Pool, d draft.Draft) string {
	t.Helper()
	stored, err := draft.Encode(d)
	if err != nil {
		t.Fatal(err)
	}
	var draftID string
	if err := pool.QueryRow(t.Context(), `select gen_random_uuid()::text`).Scan(&draftID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		insert into agent_drafts
		  (agent_id, draft_id, company_id, area_id, definition, blocks, revision,
		   created_by, created_at, updated_by, updated_at)
		values ($1, $2::uuid, $3, $4, $5, $6, 'legacy-revision',
		        'usr_ana', now(), 'usr_ana', now())`, string(d.Spec.ID), draftID,
		string(d.Spec.Company), string(d.Spec.Area), stored.Definition, stored.Blocks); err != nil {
		t.Fatalf("insert legacy draft: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		insert into agent_draft_trail
		  (draft_id, seq, agent_id, action, origin, revision, by_user, at)
		values ($1::uuid, 1, $2, 'create', 'control',
		        'legacy-revision', 'usr_ana', now())`, draftID, string(d.Spec.ID)); err != nil {
		t.Fatalf("insert legacy trail: %v", err)
	}
	return draftID
}
