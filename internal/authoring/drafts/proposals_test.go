package drafts_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The conversation's proposals (NT-018 phase 3, contract 3).

The model only proposes. A proposal is bound to its author, its thread, its
draft and the ETag it read; only its author accepting it, by its id, applies
it, and against a draft that moved since it is a 412. Its body is in the
content store, owned by the thread.
*/

func as(by domain.UserID, scopes ...domain.Scope) drafts.Caller {
	c := in(scopes...)
	c.By = by
	return c
}

// opened is a new agent's draft in cx with instructions, so block
// operations have blocks to name.
func openedWithBlocks(t *testing.T, store *drafts.Store, agent domain.AgentID) drafts.Opened {
	t.Helper()
	o, err := store.Create(t.Context(), agent, &drafts.Identity{Name: "Triagem", Scope: cx}, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return o
}

func propose(t *testing.T, store *drafts.Store, agent domain.AgentID, by domain.UserID, read domain.Revision, op draft.Operation) drafts.Proposal {
	t.Helper()
	p, err := store.Propose(t.Context(), drafts.Proposing{
		Agent: agent, Op: op, Read: read, Caller: as(by, cx), Validator: validator, Now: now,
	})
	if err != nil {
		t.Fatalf("Propose(%s): %v", op.Kind(), err)
	}
	return p
}

func accept(t *testing.T, store *drafts.Store, agent domain.AgentID, id string, c drafts.Caller) (drafts.Opened, error) {
	t.Helper()
	return store.Accept(t.Context(), drafts.Acceptance{Agent: agent, Proposal: id, Caller: c, Validator: validator, Now: now})
}

func TestPropose_bindsEachProposalToItsAuthorsThreadAndTheETagRead(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)

	a1 := propose(t, store, agent, "usr_ana", o.Revision, draft.AddTool{Tool: "crm.lookup"})
	a2 := propose(t, store, agent, "usr_ana", o.Revision, draft.SetReplyLocale{Locale: "en-US"})
	b1 := propose(t, store, agent, "usr_bia", o.Revision, draft.AddTool{Tool: "crm.reply"})

	if a1.Thread != a2.Thread || a1.Thread == b1.Thread {
		t.Fatalf("threads %s %s %s: one per author over the shared draft", a1.Thread, a2.Thread, b1.Thread)
	}
	if a1.Read != o.Revision || a1.Draft != o.DraftID || a1.Author != "usr_ana" || !a1.Valid {
		t.Errorf("proposal %+v", a1)
	}
	if mine, err := store.Proposals(t.Context(), agent, as("usr_ana", cx)); err != nil || len(mine) != 2 {
		t.Errorf("Ana's thread = %d proposals (%v), want her two and not Bia's", len(mine), err)
	}
}

// What a proposal says lives in the content store, owned by its thread, so
// erasing the thread reaches it; the row keeps only ids and digests.
func TestPropose_keepsTheBodyInTheContentStoreUnderTheThread(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	p := propose(t, store, agent, "usr_ana", o.Revision, draft.SetReplyLocale{Locale: "en-US"})

	var owner, kind, digest string
	var truncated bool
	if err := pool.QueryRow(t.Context(), `
		select c.owner_kind, c.run_id, c.digest, c.truncated
		from agent_draft_proposals p join run_content c on c.ref = p.body_ref
		where p.proposal_id = $1`, p.ID).Scan(&kind, &owner, &digest, &truncated); err != nil {
		t.Fatalf("read the body: %v", err)
	}
	if kind != "authoring" || owner != p.Thread || truncated || digest != draft.Digest(draft.SetReplyLocale{Locale: "en-US"}) {
		t.Errorf("body under %s/%s, truncated %v, digest %s", kind, owner, truncated, digest)
	}
}

func TestPropose_againstAnOlderETag_isRefused(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	if _, err := store.Apply(t.Context(), drafts.Application{Agent: agent, Op: draft.AddTool{Tool: "crm.lookup"},
		Origin: draft.FromControl, Expect: ifMatch(t, o.Revision), Caller: as("usr_ana", cx), Validator: validator, Now: now}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	_, err := store.Propose(t.Context(), drafts.Proposing{Agent: agent, Op: draft.SetReplyLocale{Locale: "en-US"},
		Read: o.Revision, Caller: as("usr_ana", cx), Validator: validator, Now: now})
	staleAt(t, err)
}

// A proposal the model may not make, or that publication would refuse, is
// kept, shown crossed out, and can never be accepted.
func TestPropose_anInvalidProposal_isKeptAndCannotBeAccepted(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	for _, op := range []draft.Operation{
		draft.SetIdentity{Name: "T", Scope: ops},
		draft.SetCachePrefixTTL{TTL: "2h"},
	} {
		p := propose(t, store, agent, "usr_ana", o.Revision, op)
		if p.Valid || p.Refusal == "" {
			t.Errorf("%s: valid %v, refusal %q; want it invalid with its reason", op.Kind(), p.Valid, p.Refusal)
		}
		if _, err := accept(t, store, agent, p.ID, as("usr_ana", cx)); !errors.Is(err, drafts.ErrProposalInvalid) {
			t.Errorf("%s: Accept = %v, want ErrProposalInvalid", op.Kind(), err)
		}
	}
}

// Accepting applies the stored operation, as a proposal: block operations
// included, which no person can apply by hand.
func TestAccept_appliesTheStoredOperationAndRecordsTheDecision(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	p := propose(t, store, agent, "usr_ana", o.Revision,
		draft.InsertBlock{At: 0, Type: blocks.Objective, Text: "Triar chamados.", Locale: "pt-BR"})

	applied, err := accept(t, store, agent, p.ID, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if !strings.Contains(applied.Draft.Definition().Instructions, "Objetivo\nTriar chamados.") {
		t.Fatalf("instructions = %q", applied.Draft.Definition().Instructions)
	}
	var decision, revision, origin, trailRef string
	if err := pool.QueryRow(t.Context(), `
		select d.decision, d.revision, t.origin, t.body_ref
		from agent_draft_proposal_decisions d
		join agent_draft_trail t on t.agent_id = $2 and t.action = 'insert_block'
		where d.proposal_id = $1`, p.ID, string(agent)).Scan(&decision, &revision, &origin, &trailRef); err != nil {
		t.Fatalf("read the decision: %v", err)
	}
	if decision != "accepted" || revision != string(applied.Revision) || origin != "proposal" || trailRef != p.BodyRef {
		t.Errorf("decision %s at %s, trail origin %s ref %s (proposal ref %s)", decision, revision, origin, trailRef, p.BodyRef)
	}
}

func TestAccept_refusals_writeNothing(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	older := propose(t, store, agent, "usr_ana", o.Revision, draft.SetReplyLocale{Locale: "en-US"})
	first := propose(t, store, agent, "usr_ana", o.Revision, draft.AddTool{Tool: "crm.lookup"})
	moved, err := accept(t, store, agent, first.ID, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Accept(first): %v", err)
	}

	if got := staleAt(t, func() error { _, err := accept(t, store, agent, older.ID, as("usr_ana", cx)); return err }()); got != moved.Revision {
		t.Errorf("a proposal made against an older draft: current = %s, want %s", got, moved.Revision)
	}
	for name, tc := range map[string]struct {
		id     string
		caller drafts.Caller
		want   error
	}{
		"another author's proposal":       {older.ID, as("usr_bia", cx), drafts.ErrNotFound},
		"out of reach, an older proposal": {older.ID, as("usr_ana", ops), drafts.ErrNotFound},
		"decided already":                 {first.ID, as("usr_ana", cx), drafts.ErrDecided},
		"no such proposal":                {"00000000-0000-0000-0000-000000000000", as("usr_ana", cx), drafts.ErrNotFound},
	} {
		if _, err := accept(t, store, agent, tc.id, tc.caller); !errors.Is(err, tc.want) {
			t.Errorf("%s: Accept = %v, want %v", name, err, tc.want)
		}
	}
	if got, _ := store.Get(t.Context(), agent, as("usr_ana", cx)); got.Revision != moved.Revision {
		t.Errorf("the refusals moved the draft to %s", got.Revision)
	}
}

func TestReject_isFinalAndChangesNothing(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	p := propose(t, store, agent, "usr_ana", o.Revision, draft.AddTool{Tool: "crm.lookup"})

	if err := store.Reject(t.Context(), agent, p.ID, as("usr_bia", cx)); !errors.Is(err, drafts.ErrNotFound) {
		t.Errorf("another author rejects = %v, want ErrNotFound", err)
	}
	if err := store.Reject(t.Context(), agent, p.ID, as("usr_ana", cx)); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if _, err := accept(t, store, agent, p.ID, as("usr_ana", cx)); !errors.Is(err, drafts.ErrDecided) {
		t.Errorf("Accept after Reject = %v, want ErrDecided", err)
	}
	if got, _ := store.Get(t.Context(), agent, as("usr_ana", cx)); got.Revision != o.Revision {
		t.Errorf("a rejection moved the draft")
	}
}

// A draft discarded and opened again is another draft: the old one's
// proposals are stale against it.
func TestAccept_aProposalOfADiscardedDraft_isStale(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	p := propose(t, store, agent, "usr_ana", o.Revision, draft.AddTool{Tool: "crm.lookup"})
	if err := store.Discard(t.Context(), agent, ifMatch(t, o.Revision), as("usr_ana", cx)); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	again := openedWithBlocks(t, store, agent)

	_, err := accept(t, store, agent, p.ID, as("usr_ana", cx))
	if got := staleAt(t, err); got != again.Revision {
		t.Errorf("current = %s, want the new draft's %s", got, again.Revision)
	}
}

// A body erased by retention or by erasing the thread leaves a proposal
// nothing can apply.
func TestAccept_aProposalWhoseBodyWasErased_isGone(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	p := propose(t, store, agent, "usr_ana", o.Revision, draft.AddTool{Tool: "crm.lookup"})
	if _, err := pool.Exec(t.Context(), `
		update run_content set bytes = '', erased_at = now(), erased_reason = 'retention' where ref = $1`, p.BodyRef); err != nil {
		t.Fatalf("erase: %v", err)
	}
	if _, err := accept(t, store, agent, p.ID, as("usr_ana", cx)); !errors.Is(err, drafts.ErrNotFound) {
		t.Fatalf("Accept = %v, want ErrNotFound", err)
	}
}

// Two proposals read the same ETag; accepted together, one lands and the
// other is stale.
func TestAccept_twoProposalsOfTheSameETagAtOnce_oneLands(t *testing.T) {
	pool := testdb.Pool(t)
	agent := uniqueAgent(t)
	seed := drafts.NewStore(pool)
	o := openedWithBlocks(t, seed, agent)
	a := propose(t, seed, agent, "usr_ana", o.Revision, draft.SetReplyLocale{Locale: "en-US"})
	b := propose(t, seed, agent, "usr_ana", o.Revision, draft.AddTool{Tool: "crm.lookup"})
	c := testdb.NewContention(t, 2)
	store := drafts.NewStore(c.Pool)
	run := func(id string) func() error {
		return func() error {
			_, err := store.Accept(context.Background(), drafts.Acceptance{Agent: agent, Proposal: id,
				Caller: as("usr_ana", cx), Validator: validator, Now: now})
			return err
		}
	}

	results := c.Overlap(t, `select 1 from agent_drafts where agent_id = $1 for update`, []any{string(agent)},
		[]func() error{run(a.ID), run(b.ID)})

	landed, stale := 0, 0
	for _, err := range results {
		var s *domain.StaleError
		switch {
		case err == nil:
			landed++
		case errors.As(err, &s):
			stale++
		default:
			t.Errorf("Accept = %v", err)
		}
	}
	if landed != 1 || stale != 1 {
		t.Fatalf("landed %d, stale %d; want one of each", landed, stale)
	}
}

func TestProposalsAndDecisions_areAppendOnly(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	p := propose(t, store, agent, "usr_ana", o.Revision, draft.AddTool{Tool: "crm.lookup"})
	if err := store.Reject(t.Context(), agent, p.ID, as("usr_ana", cx)); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	for _, statement := range []string{
		`update agent_draft_proposals set valid = false where proposal_id = $1`,
		`delete from agent_draft_proposals where proposal_id = $1`,
		`update agent_draft_proposal_decisions set decision = 'accepted' where proposal_id = $1`,
		`delete from agent_draft_proposal_decisions where proposal_id = $1`,
	} {
		assertRefused(t, pool, statement, p.ID)
	}
}

func assertRefused(t *testing.T, pool *pgxpool.Pool, statement string, arg any) {
	t.Helper()
	_, err := pool.Exec(t.Context(), statement, arg)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23001" {
		t.Errorf("%s = %v, want restrict_violation", statement, err)
	}
}
