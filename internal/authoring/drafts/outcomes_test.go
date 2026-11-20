package drafts_test

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/testdb"
)

/*
What became of the proposals of the turns a prompt shows, read in one bounded query of
metadata (#413 review): the turn, the kind, whether it was valid, its decision, and whether
the draft as it is would still accept it. Never the thread's whole history, and never a
body or a reason.
*/
func TestProposalOutcomes_areThoseOfTheTurnsAsked_withoutTheirBodies(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	old := settled(t, store, agent, "old", o.Revision, draft.SetReplyLocale{Locale: "en-US"})
	shown := settled(t, store, agent, "shown", o.Revision, draft.AddTool{Tool: "crm.lookup"}, draft.SetBudget{Budget: budget(400_000)})
	if _, err := pool.Exec(t.Context(), `update run_content set erased_at = now(), bytes = null
		where ref in (select body_ref from agent_draft_proposals where proposal_id = $1::uuid)`, shown[0].ID); err != nil {
		t.Fatalf("erase a body: %v", err)
	}

	got, err := store.ProposalOutcomes(t.Context(), agent, as("usr_ana", cx), []string{shown[0].Turn})
	if err != nil {
		t.Fatalf("ProposalOutcomes = %v", err)
	}
	if len(got) != 2 || got[0].Kind != "add_tool" || got[1].Kind != "set_budget" || !got[0].Valid {
		t.Fatalf("outcomes = %+v, want the shown turn's two proposals, a body erased or not", got)
	}
	// The draft has not moved, so only the erased body keeps the first from being accepted:
	// acceptance answers 404 for it, and the prompt must not call it waiting (#413 review).
	if !got[0].Gone || got[0].Current || got[1].Gone || !got[1].Current {
		t.Errorf("outcomes = %+v, want the erased one gone and not current, its sibling current", got)
	}
	for _, p := range got {
		if p.Turn == old[0].Turn {
			t.Errorf("a turn not asked for was read: %+v", p)
		}
	}
}

// Composed with #419: a proposal another turn's acceptance made stale is not current, so
// the prompt can say it can no longer be accepted; its own sibling still is.
func TestProposalOutcomes_aProposalAnotherTurnMadeStale_isNotCurrent(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	first := settled(t, store, agent, "first", o.Revision, draft.SetReplyLocale{Locale: "en-US"})
	second := settled(t, store, agent, "second", o.Revision, draft.SetBudget{Budget: budget(400_000)}, draft.AddTool{Tool: "crm.lookup"})
	if _, err := accept(t, store, agent, second[0].ID, as("usr_ana", cx)); err != nil {
		t.Fatalf("Accept = %v", err)
	}

	got, err := store.ProposalOutcomes(t.Context(), agent, as("usr_ana", cx), []string{first[0].Turn, second[0].Turn})
	if err != nil {
		t.Fatalf("ProposalOutcomes = %v", err)
	}
	current := map[string]bool{}
	for _, p := range got {
		current[p.Kind] = p.Current
	}
	if current["set_reply_locale"] || !current["add_tool"] || current["set_budget"] {
		t.Errorf("current = %v, want the other turn's proposal stale, the waiting sibling current, the decided one not", current)
	}
}

// The read is bounded in the database's work too, not only in what it answers: proposals
// are found by their turn through an index, not by a scan of the append-only table.
func TestProposalOutcomes_theProposalsOfATurnAreIndexedByIt(t *testing.T) {
	pool := testdb.Pool(t)
	var defs []string
	rows, err := pool.Query(t.Context(), `select indexdef from pg_indexes where tablename = 'agent_draft_proposals'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var def string
		if err := rows.Scan(&def); err != nil {
			t.Fatal(err)
		}
		defs = append(defs, def)
	}
	for _, def := range defs {
		if strings.Contains(def, "(turn_id") {
			return
		}
	}
	t.Errorf("no index leads with turn_id on agent_draft_proposals: %v", defs)
}
