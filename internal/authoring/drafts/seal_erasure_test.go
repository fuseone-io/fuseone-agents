package drafts_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

/*
Erasure reaches what a draft was sealed into (NT-018 phase 2, release 2a).

A seal is a copy of the draft's values as they stood, so erasing a thread or a subject
must reach every seal that copied a value from it, and the content of every run derived
from those seals, even when the draft itself no longer holds the value. Release 2b writes
seals; these tests write them as it will: the draft's source map copied beside the bytes.
*/

type sealed struct {
	id, ref string
}

// sealDraft seals the draft as it stands now, copying its source map, and returns the seal.
func sealDraft(t *testing.T, pool *pgxpool.Pool, o drafts.Opened, n int) sealed {
	t.Helper()
	ctx := t.Context()
	var id string
	if err := pool.QueryRow(ctx, `select gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	bytes := []byte("sealed definition " + id)
	sum := sha256.Sum256(bytes)
	version := "vseal" + hex.EncodeToString(sum[:])[:7]
	if _, err := pool.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, $2, $3)`,
		string(o.Agent), version, hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("claim: %v", err)
	}
	ref, err := ledger.NewContent(pool).PutFor(ctx, "authoring", id, int64(n), bytes)
	if err != nil {
		t.Fatalf("keep the sealed bytes: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into agent_seals (seal_id, agent_id, version_id, company_id, area_id, content_ref,
		  draft_id, draft_revision, sealed_by)
		values ($1::uuid, $2, $3, $4, $5, $6, $7::uuid, $8, 'usr_ana')`,
		id, string(o.Agent), version, string(cx.Company), string(cx.Area), ref, o.DraftID,
		string(o.Revision)); err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into agent_seal_sources (seal_id, part, trail_seq)
		select $1::uuid, part, trail_seq from agent_draft_sources where draft_id = $2::uuid`,
		id, o.DraftID); err != nil {
		t.Fatalf("seal sources: %v", err)
	}
	return sealed{id: id, ref: ref}
}

// derivedRun opens a run with the given origin and keeps some content for it.
func derivedRun(t *testing.T, pool *pgxpool.Pool, agent domain.AgentID, seal, thread string) string {
	t.Helper()
	run := domain.RunID("run_derived_" + time.Now().Format("150405.000000000"))
	payload := `{"trigger":"simulation","simulated":true,"experiment":"exp-1"`
	if seal != "" {
		payload += `,"seal_id":"` + seal + `"`
	}
	if thread != "" {
		payload += `,"authoring_thread":"` + thread + `","rehearsal":true`
	}
	if _, err := ledger.NewPostgres(pool).Append(t.Context(), domain.Step{
		RunID: run, Seq: 1, Kind: domain.StepRunStarted, Scope: cx, AgentID: agent, VersionID: "v",
		At: time.Now(), IdemKey: string(run) + ":1", Payload: []byte(payload + "}"),
	}); err != nil {
		t.Fatalf("open derived run: %v", err)
	}
	ref, err := ledger.NewContent(pool).Put(t.Context(), run, 1, []byte("output echoing the thread"))
	if err != nil {
		t.Fatalf("keep the run's output: %v", err)
	}
	return ref
}

func erased(t *testing.T, pool *pgxpool.Pool, ref string) bool {
	t.Helper()
	var gone bool
	if err := pool.QueryRow(context.Background(),
		`select erased_at is not null from run_content where ref = $1`, ref).Scan(&gone); err != nil {
		t.Fatalf("read %s: %v", ref, err)
	}
	return gone
}

func threadOf(t *testing.T, pool *pgxpool.Pool, turn string) string {
	t.Helper()
	var thread string
	if err := pool.QueryRow(t.Context(),
		`select thread_id::text from agent_draft_turns where turn_id = $1::uuid`, turn).Scan(&thread); err != nil {
		t.Fatalf("thread of %s: %v", turn, err)
	}
	return thread
}

// acceptedFromTurn settles a turn proposing op and accepts it.
func acceptedFromTurn(t *testing.T, store *drafts.Store, agent domain.AgentID, key string, read domain.Revision, op draft.Operation) (drafts.Turn, drafts.Opened) {
	t.Helper()
	turn := mustBegin(t, store, agent, "usr_ana", key, read, 100)
	settled, err := store.SettleTurn(t.Context(), drafts.Settlement{
		Agent: agent, Caller: as("usr_ana", cx), Turn: turn.ID, Cost: 10, Reply: "proposal",
		Proposed: []drafts.Proposed{{Op: op}}, Validator: validator, Now: now,
	})
	if err != nil || len(settled.Proposals) != 1 {
		t.Fatalf("settle %s = %+v (%v)", key, settled, err)
	}
	o, err := accept(t, store, agent, settled.Proposals[0].ID, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("accept %s: %v", key, err)
	}
	return turn, o
}

func TestEraseThread_reachesEverySealThatCopiedItsValueAndTheRunsDerivedFromIt(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	manualOnly := sealDraft(t, pool, o, 1)
	manualRun := derivedRun(t, pool, agent, manualOnly.id, "")

	turn, o := acceptedFromTurn(t, store, agent, "seal-thread", o.Revision, draft.AddTool{Tool: "crm.thread"})
	copied := sealDraft(t, pool, o, 2)
	copiedRun := derivedRun(t, pool, agent, copied.id, "")
	// The draft no longer holds the value; the seal still does.
	_ = applyControl(t, store, agent, o.Revision, draft.RemoveTool{Tool: "crm.thread"})
	thread := threadOf(t, pool, turn.ID)
	rehearsal := derivedRun(t, pool, agent, "", thread)

	if err := store.EraseThread(t.Context(), agent, as("usr_ana", cx)); err != nil {
		t.Fatalf("EraseThread: %v", err)
	}

	for name, ref := range map[string]string{
		"the seal that copied the value": copied.ref, "a run on that seal": copiedRun,
		"a rehearsal from the thread": rehearsal,
	} {
		if !erased(t, pool, ref) {
			t.Errorf("%s survived the thread's erasure", name)
		}
	}
	for name, ref := range map[string]string{
		"a seal of manual values only": manualOnly.ref, "a run on that seal": manualRun,
	} {
		if erased(t, pool, ref) {
			t.Errorf("%s was erased with a thread it never copied", name)
		}
	}
}

// A sealed value whose source could not be proved is cleared by any erasure of a thread of
// its draft, as the draft's own unknown values are.
func TestEraseThread_reachesASealWithAnUnknownSource(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	_ = mustBegin(t, store, agent, "usr_ana", "unknown-thread", o.Revision, 100)
	seal := sealDraft(t, pool, o, 1)
	if _, err := pool.Exec(t.Context(), `
		insert into agent_seal_sources (seal_id, part, trail_seq) values ($1::uuid, 'tools/crm.unproved', null)`,
		seal.id); err != nil {
		t.Fatalf("unknown source: %v", err)
	}
	run := derivedRun(t, pool, agent, seal.id, "")

	if err := store.EraseThread(t.Context(), agent, as("usr_ana", cx)); err != nil {
		t.Fatalf("EraseThread: %v", err)
	}

	if !erased(t, pool, seal.ref) || !erased(t, pool, run) {
		t.Errorf("seal erased=%v run erased=%v, want both", erased(t, pool, seal.ref), erased(t, pool, run))
	}
}

func TestSubjectErasure_reachesTheSealsOfTheCitedTurnsProposalsAndTheirRuns(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	cited, o := acceptedFromTurn(t, store, agent, "seal-subject-1", o.Revision, draft.AddTool{Tool: "crm.cited"})
	citedSeal := sealDraft(t, pool, o, 1)
	citedRun := derivedRun(t, pool, agent, citedSeal.id, "")
	_, o = acceptedFromTurn(t, store, agent, "seal-subject-2", o.Revision, draft.AddTool{Tool: "crm.neighbour"})
	_ = applyControl(t, store, agent, o.Revision, draft.RemoveTool{Tool: "crm.cited"})
	neighbourSeal := sealDraft(t, pool, mustGet(t, store, agent), 2)
	neighbourRun := derivedRun(t, pool, agent, neighbourSeal.id, "")
	if _, err := pool.Exec(t.Context(),
		`update agent_draft_turns set referenced_run = 'run_seal_subject' where turn_id = $1::uuid`, cited.ID); err != nil {
		t.Fatalf("reference run: %v", err)
	}

	eraseReferencedRun(t, store, pool, "run_seal_subject")

	if !erased(t, pool, citedSeal.ref) || !erased(t, pool, citedRun) {
		t.Errorf("cited seal erased=%v its run erased=%v, want both",
			erased(t, pool, citedSeal.ref), erased(t, pool, citedRun))
	}
	if erased(t, pool, neighbourSeal.ref) || erased(t, pool, neighbourRun) {
		t.Errorf("the neighbour's seal erased=%v its run erased=%v, want neither",
			erased(t, pool, neighbourSeal.ref), erased(t, pool, neighbourRun))
	}
}

// Seals are insert-only, so a search that scanned them would cost every seal ever taken,
// inside the erasure's transaction and its exclusive fence.
func TestSealsReached_startsFromTheDraft(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := t.Context()
	// Seeded and planned in one transaction that is rolled back: seals are insert-only, and
	// five thousand left in the package's database would skew every later plan.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest)
		select 'plan-agent', 'vplan' || g, repeat('a', 64) from generate_series(1, 5000) g
		on conflict do nothing;
		insert into agent_seals (seal_id, agent_id, version_id, company_id, area_id, content_ref,
		  draft_id, draft_revision, sealed_by)
		select gen_random_uuid(), 'plan-agent', 'vplan' || g, 'acme', 'cx', 'ref',
		  gen_random_uuid(), 'rev', 'usr_ana'
		from generate_series(1, 5000) g;
		insert into agent_seal_sources (seal_id, part, trail_seq)
		select seal_id, 'tools/crm.lookup', null from agent_seals where agent_id = 'plan-agent'
		on conflict do nothing;
		analyze agent_seals;
		analyze agent_seal_sources;`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rows, err := tx.Query(ctx, "explain "+drafts.SealsReachedSQL,
		"0b7c3e52-8a43-4b55-9a3f-0d6a3c5f9e11", []string{}, []string{})
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	plan := ""
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan: %v", err)
		}
		plan += line + "\n"
	}
	rows.Close()
	if strings.Contains(plan, "Seq Scan on agent_seals") || strings.Contains(plan, "Seq Scan on agent_seal_sources") {
		t.Errorf("the search reads every seal:\n%s", plan)
	}
}
