package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"github.com/fuseone/agents/internal/domain"
)

func applyDataMigration(ctx context.Context, tx pgx.Tx, version string) error {
	if version == "0117_agent_draft_provenance" {
		return seedManualDraftProvenance(ctx, tx)
	}
	return nil
}

// seedManualDraftProvenance handles the exact legacy population known at
// 0117: a draft with no accepted proposal has only manually authored values.
// Drafts with proposal trail entries remain unstamped and take the conservative
// runtime backfill path.
func seedManualDraftProvenance(ctx context.Context, tx pgx.Tx) error {
	legacy, err := legacyManualDrafts(ctx, tx)
	if err != nil {
		return err
	}
	for _, one := range legacy {
		if one.seq == 0 {
			return fmt.Errorf("draft %s has no trail", one.id)
		}
		names, err := legacyDraftSourceNames(one.definition, one.blocks)
		if err != nil {
			return fmt.Errorf("decode draft %s: %w", one.id, err)
		}
		if err := seedOneManualDraft(ctx, tx, manualDraftSeed{one.id, one.revision, one.seq, names}); err != nil {
			return err
		}
	}
	return nil
}

type legacyDraft struct {
	id, definition, revision string
	blocks                   []byte
	seq                      int64
}

func legacyManualDrafts(ctx context.Context, tx pgx.Tx) ([]legacyDraft, error) {
	rows, err := tx.Query(ctx, `
		select d.draft_id::text, d.definition, d.blocks, d.revision,
		       coalesce((select max(t.seq) from agent_draft_trail t
		                 where t.draft_id = d.draft_id), 0)
		from agent_drafts d
		where not exists (
			select 1 from agent_draft_trail t
			where t.draft_id = d.draft_id and t.origin = 'proposal'
		)
		order by d.draft_id`)
	if err != nil {
		return nil, fmt.Errorf("read legacy drafts: %w", err)
	}
	legacy, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (legacyDraft, error) {
		var one legacyDraft
		err := row.Scan(&one.id, &one.definition, &one.blocks, &one.revision, &one.seq)
		return one, err
	})
	if err != nil {
		return nil, fmt.Errorf("read legacy draft: %w", err)
	}
	return legacy, nil
}

// legacyDefinition is deliberately the persisted subset needed to name source
// rows. Keeping this wire reader in ledger avoids making migrations depend on
// authoring, spec, engine, or worker packages.
type legacyDefinition struct {
	Name, Company, Area     string
	Provider, Model, Effort string
	CacheTTL                string `yaml:"cache_prefix_ttl"`
	Locale                  string `yaml:"reply_locale"`
	Fallback                *domain.ModelPair
	Tools                   []string
	Triggers, Emits         []yaml.Node
	Steps                   []struct{ Name string }
	Budget                  legacyBudget
	MemoryLearning          *domain.MemoryLearningPolicy `yaml:"memory_learning"`
	Approvals               *domain.ApprovalPolicy
}

type legacyBudget struct {
	Micros      int64 `yaml:"micros"`
	Tokens      int64 `yaml:"tokens"`
	ToolCalls   int64 `yaml:"tool_calls"`
	Steps       int64 `yaml:"steps"`
	WallClockMS int64 `yaml:"wall_clock_ms"`
}

type legacyBlock struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func legacyDraftSourceNames(definition string, blocksJSON []byte) ([]string, error) {
	front, err := legacyFrontmatter(definition)
	if err != nil {
		return nil, err
	}
	var definitionFields legacyDefinition
	if err := yaml.Unmarshal([]byte(front), &definitionFields); err != nil {
		return nil, fmt.Errorf("frontmatter: %w", err)
	}
	var blocks []legacyBlock
	if err := json.Unmarshal(blocksJSON, &blocks); err != nil {
		return nil, fmt.Errorf("blocks: %w", err)
	}
	return legacyPartNames(definitionFields, blocks), nil
}

func legacyFrontmatter(definition string) (string, error) {
	const opening = "---\n"
	if !strings.HasPrefix(definition, opening) {
		return "", fmt.Errorf("definition has no frontmatter")
	}
	rest := strings.TrimPrefix(definition, opening)
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", fmt.Errorf("definition has no closing frontmatter")
	}
	return rest[:end], nil
}

func legacyPartNames(d legacyDefinition, blocks []legacyBlock) []string {
	names := []string{"standingApprovals"}
	add := func(name string, present bool) {
		if present {
			names = append(names, name)
		}
	}
	add("name", d.Name != "")
	add("scope", d.Company != "" || d.Area != "")
	add("model", d.Provider != "" || d.Model != "" || d.Effort != "")
	add("fallback", d.Fallback != nil && !d.Fallback.IsZero())
	add("cachePrefixTtl", d.CacheTTL != "")
	add("replyLocale", d.Locale != "")
	add("budget", d.Budget != (legacyBudget{}))
	add("triggers", len(d.Triggers) != 0)
	add("emits", len(d.Emits) != 0)
	add("memoryLearning", d.MemoryLearning != nil && d.MemoryLearning.Normalize().Enabled())
	add("approvals", d.Approvals != nil && d.Approvals.Normalize().Direct)
	for _, tool := range d.Tools {
		names = append(names, "tools/"+tool)
	}
	for _, step := range d.Steps {
		digest := sha256.Sum256([]byte(step.Name))
		names = append(names, "steps/"+hex.EncodeToString(digest[:]))
	}
	for _, block := range blocks {
		add("block/"+block.ID, block.Text != "")
	}
	sort.Strings(names)
	return names
}

type manualDraftSeed struct {
	id, revision string
	seq          int64
	names        []string
}

func seedOneManualDraft(ctx context.Context, tx pgx.Tx, seed manualDraftSeed) error {
	if _, err := tx.Exec(ctx, `
		insert into agent_draft_trail_provenance (draft_id, trail_seq, proposal_id, parts)
		values ($1::uuid, $2, null, $3::text[])`, seed.id, seed.seq, seed.names); err != nil {
		return fmt.Errorf("seed manual trail provenance of %s: %w", seed.id, err)
	}
	if _, err := tx.Exec(ctx, `
		insert into agent_draft_sources (draft_id, part, trail_seq)
		select $1::uuid, part, $2 from unnest($3::text[]) part`, seed.id, seed.seq, seed.names); err != nil {
		return fmt.Errorf("seed manual sources of %s: %w", seed.id, err)
	}
	if _, err := tx.Exec(ctx, `
		update agent_drafts set sources_revision = $3, sources_seq = $2
		where draft_id = $1::uuid`, seed.id, seed.seq, seed.revision); err != nil {
		return fmt.Errorf("stamp manual provenance of %s: %w", seed.id, err)
	}
	return nil
}
