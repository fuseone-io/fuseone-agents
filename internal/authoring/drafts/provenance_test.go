package drafts_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

func applyControl(
	t *testing.T, store *drafts.Store, agent domain.AgentID, read domain.Revision, op draft.Operation,
) drafts.Opened {
	t.Helper()
	out, err := store.Apply(t.Context(), drafts.Application{
		Agent: agent, Op: op, Origin: draft.FromControl, Expect: ifMatch(t, read),
		Caller: as("usr_ana", cx), Validator: validator, Now: now,
	})
	if err != nil {
		t.Fatalf("Apply(%s): %v", op.Kind(), err)
	}
	return out
}

func TestProvenance_manualAndProposedValuesInOneCollectionKeepSeparateSources(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	o = applyControl(t, store, agent, o.Revision, draft.AddTool{Tool: "crm.manual"})
	p := propose(t, store, agent, "usr_ana", o.Revision, draft.AddTool{Tool: "crm.proposed"})
	o, err := accept(t, store, agent, p.ID, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	_, manualProposal := sourceOf(t, pool, o.DraftID, "tools/crm.manual")
	_, proposedBy := sourceOf(t, pool, o.DraftID, "tools/crm.proposed")
	if manualProposal != nil || proposedBy == nil || *proposedBy != p.ID {
		t.Fatalf("manual source proposal=%v, proposed source=%v; want nil and %s", manualProposal, proposedBy, p.ID)
	}
	if err := store.EraseThread(t.Context(), agent, as("usr_ana", cx)); err != nil {
		t.Fatalf("EraseThread: %v", err)
	}
	got, err := store.Get(t.Context(), agent, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !slices.Equal(got.Draft.Spec.Tools, []domain.ToolID{"crm.manual"}) {
		t.Fatalf("tools after erasure = %v, want only manual", got.Draft.Spec.Tools)
	}
	if got.Draft.Spec.Name != "Triagem" || got.Draft.Spec.Company != cx.Company || got.Draft.Spec.Area != cx.Area {
		t.Fatalf("human identity changed on conversation erasure: %+v", got.Draft.Spec)
	}
}

func sourceOf(t *testing.T, pool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, draftID, part string) (int64, *string) {
	t.Helper()
	var seq int64
	var proposal *string
	if err := pool.QueryRow(t.Context(), `
		select s.trail_seq, p.proposal_id::text
		from agent_draft_sources s
		join agent_draft_trail_provenance p
		  on p.draft_id = s.draft_id and p.trail_seq = s.trail_seq
		where s.draft_id = $1::uuid and s.part = $2`, draftID, part).Scan(&seq, &proposal); err != nil {
		t.Fatalf("source of %s: %v", part, err)
	}
	return seq, proposal
}

func TestProvenance_noOpDoesNotReplaceTheSource(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	o = applyControl(t, store, agent, o.Revision, draft.SetReplyLocale{Locale: "pt-BR"})
	wantSeq, _ := sourceOf(t, pool, o.DraftID, "replyLocale")
	p := propose(t, store, agent, "usr_ana", o.Revision, draft.SetReplyLocale{Locale: "pt-BR"})
	if _, err := accept(t, store, agent, p.ID, as("usr_ana", cx)); err != nil {
		t.Fatalf("Accept no-op: %v", err)
	}
	gotSeq, gotProposal := sourceOf(t, pool, o.DraftID, "replyLocale")
	if gotSeq != wantSeq || gotProposal != nil {
		t.Fatalf("no-op source = #%d proposal %v, want manual #%d", gotSeq, gotProposal, wantSeq)
	}
}

func TestEraseThread_keepsAProposalValueThatWasOverwrittenManually(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	p := propose(t, store, agent, "usr_ana", o.Revision, draft.SetReplyLocale{Locale: "en-US"})
	o, err := accept(t, store, agent, p.ID, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	o = applyControl(t, store, agent, o.Revision, draft.SetReplyLocale{Locale: "pt-BR"})
	if err := store.EraseThread(t.Context(), agent, as("usr_ana", cx)); err != nil {
		t.Fatalf("EraseThread: %v", err)
	}
	got, err := store.Get(t.Context(), agent, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Draft.Spec.ReplyLocale != "pt-BR" {
		t.Fatalf("reply locale = %q, want the manual overwrite", got.Draft.Spec.ReplyLocale)
	}
	_, source := sourceOf(t, pool, o.DraftID, "replyLocale")
	if source != nil {
		t.Fatalf("manual overwrite still points at proposal %v", source)
	}
}

func TestEraseThread_legacyManualDraftBeforeRepairKeepsItsValues(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	publish(t, pool, agent, cx, "Objetivo\n\nTexto digitado à mão.")
	o, err := store.Create(t.Context(), agent, nil, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Create from published: %v", err)
	}
	_ = propose(t, store, agent, "usr_ana", o.Revision, draft.SetReplyLocale{Locale: "pt-BR"})
	// The state immediately after 0117 for a legacy draft, before the repair
	// job's first pass: the values and manual-only trail predate provenance.
	if _, err := pool.Exec(t.Context(),
		`delete from agent_draft_sources where draft_id = $1::uuid`, o.DraftID); err != nil {
		t.Fatalf("clear the source map: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		update agent_drafts set sources_revision = '', sources_seq = 0
		where draft_id = $1::uuid`, o.DraftID); err != nil {
		t.Fatalf("make the source map legacy: %v", err)
	}
	if err := store.EraseThread(t.Context(), agent, as("usr_ana", cx)); err != nil {
		t.Fatalf("EraseThread: %v", err)
	}
	got, err := store.Get(t.Context(), agent, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !slices.Equal(got.Draft.Spec.Tools, []domain.ToolID{"crm.lookup"}) ||
		len(got.Draft.Blocks) != 1 || got.Draft.Blocks[0].Text != "Texto digitado à mão." {
		t.Fatalf("manual values erased: tools=%v blocks=%+v", got.Draft.Spec.Tools, got.Draft.Blocks)
	}
}

func TestEraseThread_aDraftValueChangeLeavesATrailEntry(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	p := propose(t, store, agent, "usr_ana", o.Revision, draft.AddTool{Tool: "crm.erased"})
	if _, err := accept(t, store, agent, p.ID, as("usr_ana", cx)); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if err := store.EraseThread(t.Context(), agent, as("usr_ana", cx)); err != nil {
		t.Fatalf("EraseThread: %v", err)
	}
	got, err := store.Get(t.Context(), agent, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var action, origin, revision, digest, body string
	if err := pool.QueryRow(t.Context(), `
		select action, origin, revision, digest, body_ref
		from agent_draft_trail where draft_id = $1::uuid order by seq desc limit 1`,
		o.DraftID).Scan(&action, &origin, &revision, &digest, &body); err != nil {
		t.Fatalf("read erasure trail: %v", err)
	}
	if action != "erase" || origin != "control" || revision != string(got.Revision) || digest != "" || body != "" {
		t.Fatalf("erasure trail = %q/%q revision %q digest %q body %q", action, origin, revision, digest, body)
	}
}

func TestProvenance_creationAndOpeningPublishedVersionSourceEveryCurrentPart(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	created := openedWithBlocks(t, store, uniqueAgent(t))
	assertSourceParts(t, pool, created.DraftID, []string{"name", "scope", "standingApprovals"})

	agent := uniqueAgent(t)
	publish(t, pool, agent, cx, "Objetivo\n\nAtender bem.")
	opened, err := store.Create(t.Context(), agent, nil, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Create from published: %v", err)
	}
	assertSourceParts(t, pool, opened.DraftID, []string{
		"block/b1", "budget", "model", "name", "scope", "standingApprovals", "tools/crm.lookup",
	})
}

func assertSourceParts(t *testing.T, pool interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, draftID string, want []string) {
	t.Helper()
	rows, err := pool.Query(t.Context(), `
		select part from agent_draft_sources where draft_id = $1::uuid order by part`, draftID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("source parts = %v, want %v", got, want)
	}
}

func TestProvenance_staleMapMakesEveryConversationCapablePartUnknown(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	o = applyControl(t, store, agent, o.Revision, draft.AddTool{Tool: "crm.manual-before-old-writer"})
	p := propose(t, store, agent, "usr_ana", o.Revision, draft.InsertBlock{
		At: 0, Type: blocks.Objective, Text: "old replica sentinel", Locale: "pt-BR",
	})
	if _, err := accept(t, store, agent, p.ID, as("usr_ana", cx)); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	// An old writer changes revision and trail_seq without the source stamp.
	// Resetting only the stamp recreates the safety-relevant state: even a
	// surviving manual source row cannot be trusted for its current value.
	if _, err := pool.Exec(t.Context(), `
		update agent_drafts set sources_revision = '', sources_seq = 0
		where agent_id = $1`, string(agent)); err != nil {
		t.Fatalf("simulate old replica: %v", err)
	}
	if err := store.EraseThread(t.Context(), agent, as("usr_ana", cx)); err != nil {
		t.Fatalf("EraseThread: %v", err)
	}
	got, err := store.Get(t.Context(), agent, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Draft.Spec.Tools) != 0 || len(got.Draft.Blocks) != 1 || got.Draft.Blocks[0].Text != "" {
		t.Fatalf("unknown conversation values survived: tools=%v blocks=%+v", got.Draft.Spec.Tools, got.Draft.Blocks)
	}
	if got.Draft.Spec.Name != "Triagem" || got.Draft.Spec.Company != cx.Company || got.Draft.Spec.Area != cx.Area || got.Draft.Spec.StandingApprovals != domain.StandingAllowed {
		t.Fatalf("human-only values changed: %+v", got.Draft.Spec)
	}
	assertSourceParts(t, pool, got.DraftID, []string{"name", "scope", "standingApprovals"})
}

func TestProvenance_newWriterBackfillsManualValuesBeforeSourcingItsChange(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	o = applyControl(t, store, agent, o.Revision, draft.SetReplyLocale{Locale: "pt-BR"})
	if _, err := pool.Exec(t.Context(), `
		update agent_drafts set sources_revision = '', sources_seq = 0 where agent_id = $1`, string(agent)); err != nil {
		t.Fatalf("stale the map: %v", err)
	}
	o = applyControl(t, store, agent, o.Revision, draft.AddTool{Tool: "crm.new-writer"})
	assertSourceParts(t, pool, o.DraftID, []string{
		"name", "replyLocale", "scope", "standingApprovals", "tools/crm.new-writer",
	})
	// No proposal was ever accepted, so the stale locale is known to be manual.
	// An otherwise unrelated thread must not make erasure remove it.
	_ = propose(t, store, agent, "usr_ana", o.Revision, draft.SetModel{Provider: "openai", Model: "gpt-test"})
	if err := store.EraseThread(t.Context(), agent, as("usr_ana", cx)); err != nil {
		t.Fatalf("EraseThread: %v", err)
	}
	got, err := store.Get(t.Context(), agent, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Draft.Spec.ReplyLocale != "pt-BR" || !slices.Equal(got.Draft.Spec.Tools, []domain.ToolID{"crm.new-writer"}) {
		t.Fatalf("after erasure: locale=%q tools=%v", got.Draft.Spec.ReplyLocale, got.Draft.Spec.Tools)
	}
}

func TestProvenance_newWriterBackfillsAProposalValueWhoseLegacySourceLooksManual(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	o = applyControl(t, store, agent, o.Revision, draft.SetReplyLocale{Locale: "pt-BR"})
	manualSeq, _ := sourceOf(t, pool, o.DraftID, "replyLocale")
	manual := o
	p := propose(t, store, agent, "usr_ana", o.Revision, draft.SetReplyLocale{Locale: "en-US"})
	if _, err := accept(t, store, agent, p.ID, as("usr_ana", cx)); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	staleReplyLocaleSource(t, pool, manual, manualSeq)
	o = applyControl(t, store, agent, mustGet(t, store, agent).Revision,
		draft.AddTool{Tool: "crm.manual-after-old-writer"})
	if err := store.EraseThread(t.Context(), agent, as("usr_ana", cx)); err != nil {
		t.Fatalf("EraseThread: %v", err)
	}
	got := mustGet(t, store, agent)
	if got.Draft.Spec.ReplyLocale != "" ||
		!slices.Equal(got.Draft.Spec.Tools, []domain.ToolID{"crm.manual-after-old-writer"}) {
		t.Fatalf("after erasure: locale=%q tools=%v", got.Draft.Spec.ReplyLocale, got.Draft.Spec.Tools)
	}
}

func staleReplyLocaleSource(t *testing.T, pool *pgxpool.Pool, manual drafts.Opened, seq int64) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		update agent_draft_sources set trail_seq = $2
		where draft_id = $1::uuid and part = 'replyLocale'`, manual.DraftID, seq); err != nil {
		t.Fatalf("restore the legacy locale source: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		update agent_drafts set sources_revision = $2, sources_seq = $3
		where draft_id = $1::uuid`, manual.DraftID, string(manual.Revision), seq); err != nil {
		t.Fatalf("restore the legacy source stamp: %v", err)
	}
}

func TestEraseThread_removesAcceptedSentinelFromTranscriptBodyAndDraftValue(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	turn := mustBegin(t, store, agent, "usr_ana", "sentinel", o.Revision, 100)
	const sentinel = "SENTINEL-PROVENANCE-018"
	settled, err := store.SettleTurn(t.Context(), drafts.Settlement{
		Agent: agent, Caller: as("usr_ana", cx), Turn: turn.ID, Cost: 10, Reply: sentinel,
		Proposed:  []drafts.Proposed{{Op: draft.InsertBlock{At: 0, Type: blocks.Objective, Text: sentinel, Locale: "pt-BR"}, Reason: sentinel}},
		Validator: validator, Now: now,
	})
	if err != nil || len(settled.Proposals) != 1 {
		t.Fatalf("SettleTurn = %+v (%v)", settled, err)
	}
	if _, err := accept(t, store, agent, settled.Proposals[0].ID, as("usr_ana", cx)); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if err := store.EraseThread(t.Context(), agent, as("usr_ana", cx)); err != nil {
		t.Fatalf("EraseThread: %v", err)
	}
	got, err := store.Get(t.Context(), agent, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Draft.Blocks) != 1 || got.Draft.Blocks[0].Text != "" {
		t.Fatalf("blocks after erasure = %+v, want one empty marker", got.Draft.Blocks)
	}
	var kept int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from run_content
		where owner_kind = 'authoring' and run_id = $1 and erased_at is null`, turn.Thread).Scan(&kept); err != nil || kept != 0 {
		t.Fatalf("kept authoring content = %d (%v), want zero", kept, err)
	}
}

func TestRepairProvenance_reappliesAnOldThreadErasure(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	turn := mustBegin(t, store, agent, "usr_ana", "old-erasure", o.Revision, 100)
	settled, err := store.SettleTurn(t.Context(), drafts.Settlement{
		Agent: agent, Caller: as("usr_ana", cx), Turn: turn.ID, Cost: 10, Reply: "old",
		Proposed: []drafts.Proposed{{Op: draft.AddTool{Tool: "crm.old-proposal"}}}, Validator: validator, Now: now,
	})
	if err != nil || len(settled.Proposals) != 1 {
		t.Fatalf("SettleTurn = %+v (%v)", settled, err)
	}
	if _, err := accept(t, store, agent, settled.Proposals[0].ID, as("usr_ana", cx)); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	// What the previous binary left: the turn and content are erased, but the
	// value and reconciliation marker are untouched.
	if _, err := pool.Exec(t.Context(), `update agent_draft_turns set erased = true where turn_id = $1::uuid`, turn.ID); err != nil {
		t.Fatalf("simulate old erasure: %v", err)
	}
	if _, err := store.RepairProvenance(t.Context()); err != nil {
		t.Fatalf("RepairProvenance: %v", err)
	}
	got, err := store.Get(t.Context(), agent, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if slices.Contains(got.Draft.Spec.Tools, domain.ToolID("crm.old-proposal")) {
		t.Fatalf("old proposal value survived repair: %v", got.Draft.Spec.Tools)
	}
}

func TestRepairProvenance_oneBrokenDraftDoesNotHoldTheFollowingDrafts(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	badAgent := domain.AgentID("a-bad-" + string(uniqueAgent(t)))
	bad := draft.New(badAgent, "Bad", cx)
	stored, err := draft.Encode(bad)
	if err != nil {
		t.Fatal(err)
	}
	var badID string
	if err := pool.QueryRow(t.Context(), `select gen_random_uuid()::text`).Scan(&badID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		insert into agent_drafts
		  (agent_id, draft_id, company_id, area_id, definition, blocks, revision,
		   created_by, created_at, updated_by, updated_at)
		values ($1, $2::uuid, $3, $4, $5, $6, 'bad-revision', 'usr', now(), 'usr', now())`,
		string(badAgent), badID, string(cx.Company), string(cx.Area), stored.Definition, stored.Blocks); err != nil {
		t.Fatalf("insert broken draft: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		insert into agent_draft_trail
		  (draft_id, seq, agent_id, action, origin, revision, by_user, at)
		values ($2::uuid, 1, $1, 'set_model', 'proposal', 'bad-revision', 'usr', now())`,
		string(badAgent), badID); err != nil {
		t.Fatalf("insert broken trail: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := pool.Exec(ctx, `
			insert into agent_draft_trail
			  (draft_id, seq, agent_id, action, origin, revision, by_user, at)
			values ($2::uuid, 2, $1, 'create', 'control', 'bad-revision', 'usr', now())
			on conflict do nothing`, string(badAgent), badID); err != nil {
			t.Errorf("make broken draft repairable: %v", err)
		}
	})

	goodAgent := domain.AgentID("z-good-" + string(uniqueAgent(t)))
	good := openedWithBlocks(t, store, goodAgent)
	if _, err := pool.Exec(t.Context(),
		`delete from agent_draft_sources where draft_id = $1::uuid`, good.DraftID); err != nil {
		t.Fatalf("clear following draft sources: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		update agent_drafts set sources_revision = '', sources_seq = 0
		where draft_id = $1::uuid`, good.DraftID); err != nil {
		t.Fatalf("make following draft need repair: %v", err)
	}
	if _, err := store.RepairProvenance(t.Context()); err == nil {
		t.Fatal("RepairProvenance succeeded despite the broken first draft")
	}
	var sources int
	var stamped bool
	if err := pool.QueryRow(t.Context(), `
		select (select count(*) from agent_draft_sources where draft_id = d.draft_id),
		       d.sources_revision = d.revision and d.sources_seq = (
		         select max(seq) from agent_draft_trail where draft_id = d.draft_id)
		from agent_drafts d where draft_id = $1::uuid`, good.DraftID).Scan(&sources, &stamped); err != nil {
		t.Fatal(err)
	}
	if sources == 0 || !stamped {
		t.Fatalf("following draft was not repaired: sources=%d stamped=%v", sources, stamped)
	}
}

func TestSubjectErasure_onlyReachesTheTurnThatReferencesTheRun(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	first := mustBegin(t, store, agent, "usr_ana", "subject-1", o.Revision, 100)
	settled, err := store.SettleTurn(t.Context(), drafts.Settlement{
		Agent: agent, Caller: as("usr_ana", cx), Turn: first.ID, Cost: 10, Reply: "erase me",
		Proposed: []drafts.Proposed{{Op: draft.AddTool{Tool: "crm.subject"}}}, Validator: validator, Now: now,
	})
	if err != nil || len(settled.Proposals) != 1 {
		t.Fatalf("settle first = %+v (%v)", settled, err)
	}
	o, err = accept(t, store, agent, settled.Proposals[0].ID, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("accept first: %v", err)
	}
	second := mustBegin(t, store, agent, "usr_ana", "subject-2", o.Revision, 100)
	settled, err = store.SettleTurn(t.Context(), drafts.Settlement{
		Agent: agent, Caller: as("usr_ana", cx), Turn: second.ID, Cost: 10, Reply: "keep me",
		Proposed: []drafts.Proposed{{Op: draft.AddTool{Tool: "crm.keep"}}}, Validator: validator, Now: now,
	})
	if err != nil || len(settled.Proposals) != 1 {
		t.Fatalf("settle second = %+v (%v)", settled, err)
	}
	if _, err := accept(t, store, agent, settled.Proposals[0].ID, as("usr_ana", cx)); err != nil {
		t.Fatalf("accept second: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `update agent_draft_turns set referenced_run = 'run_subject' where turn_id = $1::uuid`, first.ID); err != nil {
		t.Fatalf("reference run: %v", err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EraseReferencedRunsTx(t.Context(), tx, []domain.RunID{"run_subject"}, "usr_admin"); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatalf("EraseReferencedRunsTx: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit: %v", err)
	}
	got, err := store.Get(t.Context(), agent, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !slices.Equal(got.Draft.Spec.Tools, []domain.ToolID{"crm.keep"}) {
		t.Fatalf("tools after subject erasure = %v, want only the other turn's value", got.Draft.Spec.Tools)
	}
	turns, err := store.Turns(t.Context(), agent, as("usr_ana", cx))
	if err != nil || len(turns) != 2 || !turns[0].Erased || turns[1].Erased || turns[1].Reply != "keep me" {
		t.Fatalf("turns after subject erasure = %+v (%v)", turns, err)
	}
}

func TestSubjectErasure_staleMapDoesNotTrustAProposalValueWithAnOldManualSource(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	o = applyControl(t, store, agent, o.Revision, draft.SetReplyLocale{Locale: "pt-BR"})
	manualSeq, _ := sourceOf(t, pool, o.DraftID, "replyLocale")
	manual := o
	turn := mustBegin(t, store, agent, "usr_ana", "subject-stale", o.Revision, 100)
	settled, err := store.SettleTurn(t.Context(), drafts.Settlement{
		Agent: agent, Caller: as("usr_ana", cx), Turn: turn.ID, Cost: 10, Reply: "proposal",
		Proposed: []drafts.Proposed{{Op: draft.SetReplyLocale{Locale: "en-US"}}}, Validator: validator, Now: now,
	})
	if err != nil || len(settled.Proposals) != 1 {
		t.Fatalf("SettleTurn = %+v (%v)", settled, err)
	}
	if _, err := accept(t, store, agent, settled.Proposals[0].ID, as("usr_ana", cx)); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	staleReplyLocaleSource(t, pool, manual, manualSeq)
	if _, err := pool.Exec(t.Context(),
		`update agent_draft_turns set referenced_run = 'run_subject_stale' where turn_id = $1::uuid`, turn.ID); err != nil {
		t.Fatalf("reference run: %v", err)
	}
	eraseReferencedRun(t, store, pool, "run_subject_stale")
	if got := mustGet(t, store, agent).Draft.Spec.ReplyLocale; got != "" {
		t.Fatalf("reply locale after subject erasure = %q, want empty", got)
	}
}

func eraseReferencedRun(t *testing.T, store *drafts.Store, pool *pgxpool.Pool, run domain.RunID) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.EraseReferencedRunsTx(t.Context(), tx, []domain.RunID{run}, "usr_admin"); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatalf("EraseReferencedRunsTx: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

type blockedCorpusFence struct {
	entered chan struct{}
	release chan struct{}
}

func (f *blockedCorpusFence) Exclusive(
	ctx context.Context, _ []domain.AgentID, inside func(context.Context) error,
) error {
	close(f.entered)
	select {
	case <-f.release:
		return inside(ctx)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestEraseThread_waitsForTheCorpusFenceBeforeHoldingTheDraftRow(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	openedWithBlocks(t, store, agent)
	_ = propose(t, store, agent, "usr_ana", mustGet(t, store, agent).Revision,
		draft.SetReplyLocale{Locale: "pt-BR"})
	fence := &blockedCorpusFence{entered: make(chan struct{}), release: make(chan struct{})}
	store.WithCorpusFence(fence)
	done := make(chan error, 1)
	go func() { done <- store.EraseThread(context.Background(), agent, as("usr_ana", cx)) }()
	select {
	case <-fence.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("EraseThread did not reach the corpus fence")
	}

	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var locked string
	if err := tx.QueryRow(t.Context(), `
		select agent_id from agent_drafts where agent_id = $1 for update nowait`, string(agent)).Scan(&locked); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatalf("draft row was held while waiting for the fence: %v", err)
	}
	_ = tx.Rollback(t.Context())
	close(fence.release)
	if err := <-done; err != nil {
		t.Fatalf("EraseThread: %v", err)
	}
}

func mustGet(t *testing.T, store *drafts.Store, agent domain.AgentID) drafts.Opened {
	t.Helper()
	o, err := store.Get(t.Context(), agent, as("usr_ana", cx))
	if err != nil {
		t.Fatal(err)
	}
	return o
}
