package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/publishable"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
A proposal from the model to the draft, through the router and Postgres (NT-018
phase 3). The model proposes in Ana's turn; Ana reads her own thread, accepts
by the id alone, and the draft moves; a proposal made against the draft before
that is a 412, and nobody but Ana decides hers.
*/
func TestProposalRoute_aProposalIsAppliedOnlyByItsAuthorAcceptingIt(t *testing.T) {
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	api := httpapi.NewServer(ledger.NewMemory(), "test").
		WithAgents(spec.NewRegistry(pool)).
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{})).
		WithDrafts(store)
	route := draftJourney{etagJourney{handler: apiHandlerFor(api)}}
	agent := fmt.Sprintf("nt018-proposal-%d", time.Now().UnixNano())
	ana, bia := areaAuthor("factory"), domain.Principal{ID: "usr_bia", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "factory"}, Role: domain.RoleAuthor}}}

	created := route.create(t, ana, agent, "factory")
	wantStatus(t, "create", created, http.StatusCreated)
	read := domain.Revision(created.Header().Get("ETag"))
	propose := func(op draft.Operation) string {
		p, err := store.Propose(t.Context(), drafts.Proposing{Agent: domain.AgentID(agent), Op: op, Read: read,
			Caller:    drafts.Caller{By: "usr_ana", Allow: func(domain.Scope) bool { return true }},
			Validator: draft.Validator{Checker: publishable.Checker{}}, Now: time.Now()})
		if err != nil {
			t.Fatalf("Propose: %v", err)
		}
		return p.ID
	}
	block := propose(draft.InsertBlock{At: 0, Type: blocks.Objective, Text: "Triar chamados.", Locale: "pt-BR"})
	older := propose(draft.SetReplyLocale{Locale: "en-US"})

	var thread struct{ Items []openapi.DraftProposal }
	decode(t, route.do(t, ana, http.MethodGet, "/api/v1/agents/"+agent+"/draft/proposals", nil, nil), &thread)
	if len(thread.Items) != 2 || thread.Items[0].Kind != "insert_block" {
		t.Fatalf("Ana's thread = %+v", thread.Items)
	}
	decode(t, route.do(t, bia, http.MethodGet, "/api/v1/agents/"+agent+"/draft/proposals", nil, nil), &thread)
	if len(thread.Items) != 0 {
		t.Errorf("Bia reads %d of Ana's proposals", len(thread.Items))
	}

	acceptPath := func(id string) string { return "/api/v1/agents/" + agent + "/draft/proposals/" + id + "/accept" }
	wantStatus(t, "Bia accepts Ana's", route.do(t, bia, http.MethodPost, acceptPath(block), nil, nil), http.StatusNotFound)
	accepted := route.do(t, ana, http.MethodPost, acceptPath(block), nil, nil)
	wantStatus(t, "Ana accepts", accepted, http.StatusOK)
	var view openapi.AgentDraft
	decode(t, accepted, &view)
	if view.Definition.Instructions != "Objetivo\nTriar chamados." {
		t.Errorf("instructions = %q", view.Definition.Instructions)
	}
	wantStatus(t, "accepted again", route.do(t, ana, http.MethodPost, acceptPath(block), nil, nil), http.StatusConflict)
	stale := route.do(t, ana, http.MethodPost, acceptPath(older), nil, nil)
	if wantStatus(t, "a proposal of the older draft", stale, http.StatusPreconditionFailed); stale.Header().Get("ETag") != accepted.Header().Get("ETag") {
		t.Errorf("412 names %q, want the draft's %q", stale.Header().Get("ETag"), accepted.Header().Get("ETag"))
	}
	wantStatus(t, "Ana rejects it", route.do(t, ana, http.MethodPost, "/api/v1/agents/"+agent+"/draft/proposals/"+older+"/reject", nil, nil), http.StatusNoContent)
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, into any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		t.Fatalf("decode: %v", err)
	}
}
