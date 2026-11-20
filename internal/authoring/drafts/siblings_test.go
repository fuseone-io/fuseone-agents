package drafts_test

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
)

/*
Proposals one turn made are accepted in any order, one after another (#412). Every one
of them names the ETag the model read, and the first acceptance moves the draft, so
judging each against that ETag alone left every sibling stale by construction: the
author could take one card of a turn and never the rest.

A sibling is still judged on the draft as it is, and a draft moved by anything other than
its own turn's acceptances is still stale (NT-018, contract 3).
*/

// settled is one turn of Ana's over the draft at read, proposing ops.
func settled(t *testing.T, store *drafts.Store, agent domain.AgentID, key string, read domain.Revision, ops ...draft.Operation) []drafts.Proposal {
	t.Helper()
	turn := mustBegin(t, store, agent, "usr_ana", key, read, 100)
	proposed := make([]drafts.Proposed, 0, len(ops))
	for _, op := range ops {
		proposed = append(proposed, drafts.Proposed{Op: op})
	}
	out, err := store.SettleTurn(t.Context(), drafts.Settlement{
		Agent: agent, Caller: as("usr_ana", cx), Turn: turn.ID, Cost: 10, Reply: "ok",
		Proposed: proposed, Validator: validator, Now: now,
	})
	if err != nil || len(out.Proposals) != len(ops) {
		t.Fatalf("SettleTurn = %+v (%v), want %d proposals", out, err, len(ops))
	}
	return out.Proposals
}

func TestAccept_theProposalsOfOneTurn_landInAnyOrder(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	turn := settled(t, store, agent, "siblings", o.Revision,
		draft.SetReplyLocale{Locale: "en-US"}, draft.AddTool{Tool: "crm.lookup"}, draft.SetBudget{Budget: budget(500_000)})

	var last drafts.Opened
	for _, i := range []int{2, 0, 1} {
		applied, err := accept(t, store, agent, turn[i].ID, as("usr_ana", cx))
		if err != nil {
			t.Fatalf("Accept(%s) = %v, want every sibling to land", turn[i].Kind, err)
		}
		last = applied
	}
	got := last.Draft.Definition()
	if got.ReplyLocale != "en-US" || !slices.Contains(got.Tools, "crm.lookup") || got.Budget.Micros != 500_000 {
		t.Errorf("draft = locale %q tools %v budget %+v, want all three proposals applied", got.ReplyLocale, got.Tools, got.Budget)
	}
}

// A change by hand between two acceptances is not the turn's: the draft is no longer one
// the model read, or one its own proposals made of it.
func TestAccept_aSiblingAfterAChangeByHand_isStale(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	turn := settled(t, store, agent, "by-hand", o.Revision,
		draft.SetReplyLocale{Locale: "en-US"}, draft.AddTool{Tool: "crm.lookup"})
	first, err := accept(t, store, agent, turn[0].ID, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Accept(first) = %v", err)
	}
	moved := applyControl(t, store, agent, first.Revision, draft.SetBudget{Budget: budget(400_000)})

	_, err = accept(t, store, agent, turn[1].ID, as("usr_ana", cx))
	if got := staleAt(t, err); got != moved.Revision {
		t.Errorf("current = %s, want %s", got, moved.Revision)
	}
}

// Another turn's acceptance is not this turn's either, though both read the same draft.
func TestAccept_aSiblingAfterAnotherTurnsProposal_isStale(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	first := settled(t, store, agent, "first", o.Revision, draft.SetReplyLocale{Locale: "en-US"})
	second := settled(t, store, agent, "second", o.Revision, draft.SetBudget{Budget: budget(400_000)})
	moved, err := accept(t, store, agent, second[0].ID, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Accept(second) = %v, want it on the draft it read", err)
	}

	_, err = accept(t, store, agent, first[0].ID, as("usr_ana", cx))
	if got := staleAt(t, err); got != moved.Revision {
		t.Errorf("current = %s, want %s", got, moved.Revision)
	}
}

func budget(micros int64) domain.Budget { return domain.Budget{Micros: micros, Steps: 60} }

// The thread says which waiting proposals the draft as it is would accept, so a card shows
// what accepting a sibling leaves instead of calling it stale.
func TestProposals_aSiblingOfAnAcceptedProposal_isStillCurrent(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	agent := uniqueAgent(t)
	o := openedWithBlocks(t, store, agent)
	turn := settled(t, store, agent, "current", o.Revision,
		draft.SetReplyLocale{Locale: "en-US"}, draft.AddTool{Tool: "crm.lookup"})
	other := settled(t, store, agent, "other", o.Revision, draft.SetBudget{Budget: budget(400_000)})
	if _, err := accept(t, store, agent, turn[0].ID, as("usr_ana", cx)); err != nil {
		t.Fatalf("Accept = %v", err)
	}

	thread, err := store.Proposals(t.Context(), agent, as("usr_ana", cx))
	if err != nil {
		t.Fatalf("Proposals = %v", err)
	}
	current := map[string]bool{}
	for _, p := range thread {
		current[p.ID] = p.Current
	}
	if !current[turn[1].ID] || current[other[0].ID] || current[turn[0].ID] {
		t.Errorf("current = %v, want the waiting sibling alone (sibling %s, other turn %s, decided %s)",
			current, turn[1].ID, other[0].ID, turn[0].ID)
	}
}
