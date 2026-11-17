package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The workbench's draft through the router, the JSON on the wire and the
Postgres store (NT-018 1b, part 3).

Each crossing could drop what the draft depends on: a header the contract did
not bind, an ETag the read and the write compute differently, an operation
that dies between the wire and the store, a refusal that answers the wrong
status. The client here is what the workbench will be.
*/

type draftJourney struct{ etagJourney }

func draftRoute(t *testing.T) draftJourney {
	t.Helper()
	pool := testdb.Pool(t)
	api := httpapi.NewServer(ledger.NewMemory(), "test").
		WithAgents(spec.NewRegistry(pool)).
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{})).
		WithRetirements(spec.NewState(pool)).
		WithDrafts(drafts.NewStore(pool))
	return draftJourney{etagJourney{handler: apiHandlerFor(api)}}
}

func (r draftJourney) create(t *testing.T, caller domain.Principal, agent, area string) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(nil)
	if area != "" {
		body = []byte(fmt.Sprintf(`{"name":"Triagem","company":"acme","area":%q}`, area))
	}
	return r.do(t, caller, http.MethodPost, "/api/v1/agents/"+agent+"/draft", body,
		map[string]string{"If-None-Match": "*"})
}

func (r draftJourney) apply(t *testing.T, caller domain.Principal, agent, ifMatch, op string) *httptest.ResponseRecorder {
	t.Helper()
	return r.do(t, caller, http.MethodPost, "/api/v1/agents/"+agent+"/draft/operations", []byte(op),
		map[string]string{"If-Match": ifMatch})
}

func (r draftJourney) read(t *testing.T, caller domain.Principal, agent string) *httptest.ResponseRecorder {
	t.Helper()
	return r.do(t, caller, http.MethodGet, "/api/v1/agents/"+agent+"/draft", nil, nil)
}

func wantStatus(t *testing.T, what string, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("%s = %d: %s; want %d", what, rec.Code, rec.Body, status)
	}
}

func TestDraftRoute_aNewAgent_isBornChangedAndDiscardedAgainstItsTag(t *testing.T) {
	route := draftRoute(t)
	agent := fmt.Sprintf("nt018-draft-%d", time.Now().UnixNano())
	author, outsider := areaAuthor("factory"), areaAuthor("billing")

	created := route.create(t, author, agent, "factory")
	wantStatus(t, "create", created, http.StatusCreated)
	born := created.Header().Get("ETag")
	again := route.create(t, author, agent, "factory")
	if wantStatus(t, "create again", again, http.StatusPreconditionFailed); again.Header().Get("ETag") != born {
		t.Errorf("the second create names %q, want the draft's %q", again.Header().Get("ETag"), born)
	}
	if read := route.read(t, author, agent); read.Header().Get("ETag") != born {
		t.Errorf("GET names %q, want %q", read.Header().Get("ETag"), born)
	}
	// Invisible to everything the published agent is read through.
	wantStatus(t, "GET the agent", route.do(t, author, http.MethodGet, "/api/v1/agents/"+agent, nil, nil), http.StatusNotFound)

	changed := route.apply(t, author, agent, born, `{"kind":"add_tool","tool":"crm.lookup"}`)
	wantStatus(t, "add_tool", changed, http.StatusOK)
	var view openapi.AgentDraft
	if err := json.Unmarshal(changed.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Definition.Tools == nil || strings.Join(*view.Definition.Tools, ",") != "crm.lookup" {
		t.Errorf("tools = %v, want crm.lookup", view.Definition.Tools)
	}
	now := changed.Header().Get("ETag")
	stale := route.apply(t, author, agent, born, `{"kind":"set_reply_locale","replyLocale":"en-US"}`)
	if wantStatus(t, "an old tag", stale, http.StatusPreconditionFailed); stale.Header().Get("ETag") != now {
		t.Errorf("412 names %q, want the current %q", stale.Header().Get("ETag"), now)
	}

	wantStatus(t, "a block by hand", route.apply(t, author, agent, now, `{"kind":"remove_block"}`), http.StatusBadRequest)
	wantStatus(t, "no If-Match", route.apply(t, author, agent, "", `{"kind":"add_tool","tool":"crm.reply"}`), http.StatusBadRequest)
	wantStatus(t, "a cache window nobody sells",
		route.apply(t, author, agent, now, `{"kind":"set_cache_prefix_ttl","cachePrefixTtl":"2h"}`), http.StatusBadRequest)
	wantStatus(t, "moved to an area not held",
		route.apply(t, author, agent, now, `{"kind":"set_identity","name":"T","company":"acme","area":"billing"}`), http.StatusForbidden)

	wantStatus(t, "an outsider reads", route.read(t, outsider, agent), http.StatusNotFound)
	wantStatus(t, "an outsider with an old tag", route.apply(t, outsider, agent, born, `{"kind":"add_tool","tool":"crm.reply"}`), http.StatusNotFound)
	wantStatus(t, "an outsider discards", route.do(t, outsider, http.MethodDelete, "/api/v1/agents/"+agent+"/draft", nil,
		map[string]string{"If-Match": now}), http.StatusNotFound)

	wantStatus(t, "discard", route.do(t, author, http.MethodDelete, "/api/v1/agents/"+agent+"/draft", nil,
		map[string]string{"If-Match": now}), http.StatusNoContent)
	wantStatus(t, "GET after discard", route.read(t, author, agent), http.StatusNotFound)
}

/*
A published agent's draft opens from its current version, untouched, and
follows the agent: moved to another area while the draft is open, the draft
needs both scopes, and an author of the old area alone no longer reaches it.
*/
func TestDraftRoute_aPublishedAgent_opensUntouchedAndNeedsBothScopesOnceMoved(t *testing.T) {
	route := draftRoute(t)
	agent := fmt.Sprintf("nt018-draft-%d", time.Now().UnixNano())
	author := areaAuthor("factory")
	published := route.put(t, author, agent, "", "", "Objetivo\nTriar.\n\nNunca\nPrometer.")
	wantStatus(t, "publish", published, http.StatusOK)

	wantStatus(t, "an identity for a published agent", route.create(t, author, agent, "factory"), http.StatusConflict)
	opened := route.create(t, author, agent, "")
	wantStatus(t, "open from the agent", opened, http.StatusCreated)
	var view openapi.AgentDraft
	if err := json.Unmarshal(opened.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Definition.Instructions != "Objetivo\nTriar.\n\nNunca\nPrometer." || view.BaseVersionId == nil || len(view.Blocks) != 2 {
		t.Fatalf("opened %+v", view)
	}

	everywhere := domain.Principal{ID: "usr_ops", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme"}, Role: domain.RoleAuthor}}}
	wantStatus(t, "move to billing", route.putIn(t, everywhere, "billing", agent, "", "", "Triar em billing."), http.StatusOK)

	wantStatus(t, "the old area's author reads", route.read(t, author, agent), http.StatusNotFound)
	wantStatus(t, "an author of both reads", route.read(t, everywhere, agent), http.StatusOK)
}
