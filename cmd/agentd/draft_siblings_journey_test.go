package main

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/publishable"
)

/*
Every card of one turn is accepted, one after another, through the route the console
calls (#412). The second card shows what accepting it would leave of the draft the first
one moved, and accepting it lands.
*/
func TestDraftProposals_theCardsOfOneTurn_areAcceptedOneAfterAnother(t *testing.T) {
	w := newPublishWorld(t)
	tag := w.ready(t)
	caller := drafts.Caller{By: "usr_ana", Allow: func(domain.Scope) bool { return true }}
	started, err := w.store.BeginTurn(t.Context(), drafts.Beginning{
		Agent: domain.AgentID(w.agent), Key: "two-cards", Message: "Diga o que nunca fazer.", Read: domain.Revision(tag),
		Reserve: 1, DailyMicros: math.MaxInt64, Lease: time.Minute, Caller: caller,
	})
	if err != nil {
		t.Fatalf("BeginTurn: %v", err)
	}
	settled, err := w.store.SettleTurn(t.Context(), drafts.Settlement{
		Agent: domain.AgentID(w.agent), Caller: caller, Turn: started.Turn.ID, Cost: 1, Reply: "Proponho dois blocos.",
		Proposed: []drafts.Proposed{
			{Op: draft.InsertBlock{At: 1, Type: blocks.Never, Text: "Prometer prazo.", Locale: "pt-BR"}},
			{Op: draft.InsertBlock{At: 1, Type: blocks.WhenToStop, Text: "Chamado resolvido.", Locale: "pt-BR"}},
		},
		Validator: draft.Validator{Checker: publishable.Checker{}}, Now: time.Now(),
	})
	if err != nil || len(settled.Proposals) != 2 {
		t.Fatalf("SettleTurn = %+v (%v), want two proposals", settled, err)
	}
	first, second := settled.Proposals[0].ID, settled.Proposals[1].ID
	accept := func(id string) *http.Response {
		rec := w.do(t, areaAuthor("factory"), http.MethodPost, "/api/v1/agents/"+w.agent+"/draft/proposals/"+id+"/accept", nil, nil)
		return rec.Result()
	}

	if got := accept(first); got.StatusCode != http.StatusOK {
		t.Fatalf("accept the first card = %d", got.StatusCode)
	}
	list := w.do(t, areaAuthor("factory"), http.MethodGet, "/api/v1/agents/"+w.agent+"/draft/proposals", nil, nil)
	wantStatus(t, "list", list, http.StatusOK)
	var thread openapi.ListDraftProposals200JSONResponse
	if err := json.Unmarshal(list.Body.Bytes(), &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, p := range thread.Items {
		if p.ProposalId == second && (p.InstructionsAfter == nil || !strings.Contains(*p.InstructionsAfter, "Prometer prazo.")) {
			t.Errorf("the second card leaves %v, want its effect on the draft the first moved", p.InstructionsAfter)
		}
	}
	if got := accept(second); got.StatusCode != http.StatusOK {
		t.Errorf("accept the second card = %d, want it to land", got.StatusCode)
	}
}
