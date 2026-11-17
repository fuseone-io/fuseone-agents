package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
The agent's entity-tag, from the read to the writes that honour it, through
the router and the Postgres publisher and state (NT-018 1a).

Each crossing could drop it: a header the contract did not bind, a tag the
read computes differently from the lock, a store that judged before it locked.
The client here does what the Terraform provider and, from 1b, the workbench
will: read, write with If-Match, and be refused once somebody else wrote.
*/
func TestAgentRoute_writesHonourTheTagTheReadGave(t *testing.T) {
	route := etagRoute(t)
	agent := fmt.Sprintf("nt018-etag-%d", time.Now().UnixNano())
	author := areaAuthor("factory")

	if rec := route.put(t, author, agent, "", "*", "Do the work."); rec.Code != http.StatusOK {
		t.Fatalf("create with If-None-Match: * = %d: %s", rec.Code, rec.Body)
	}
	if rec := route.put(t, author, agent, "", "*", "Do the work again."); rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("second create = %d: %s; want 412", rec.Code, rec.Body)
	}
	first := route.tag(t, author, agent)

	if rec := route.put(t, author, agent, first, "", "Do the work twice."); rec.Code != http.StatusOK {
		t.Fatalf("publish with the read's tag = %d: %s", rec.Code, rec.Body)
	}
	second := route.tag(t, author, agent)
	stale := route.put(t, author, agent, first, "", "Do the work thrice.")
	if stale.Code != http.StatusPreconditionFailed || stale.Header().Get("ETag") != second {
		t.Fatalf("publish with the old tag = %d (ETag %q): %s; want 412 naming %s",
			stale.Code, stale.Header().Get("ETag"), stale.Body, second)
	}

	if rec := route.retire(t, author, agent, first); rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("retire with the old tag = %d: %s; want 412", rec.Code, rec.Body)
	}
	if rec := route.retire(t, author, agent, second); rec.Code != http.StatusNoContent {
		t.Fatalf("retire with the current tag = %d: %s", rec.Code, rec.Body)
	}
	if route.tag(t, author, agent) == second {
		t.Error("retiring left the tag where it was")
	}
}

/*
An author of another area names it as the destination: before 1a that moved
the agent out of an area the author never held. Now it is the 404 of an agent
the caller cannot reach, with or without a precondition.
*/
func TestAgentRoute_movingAnAgentOutOfAnAreaTheCallerCannotPublish_isNotFound(t *testing.T) {
	route := etagRoute(t)
	agent := fmt.Sprintf("nt018-etag-%d", time.Now().UnixNano())
	if rec := route.put(t, areaAuthor("factory"), agent, "", "", "Do the work."); rec.Code != http.StatusOK {
		t.Fatalf("publish in factory = %d: %s", rec.Code, rec.Body)
	}

	rec := route.putIn(t, areaAuthor("billing"), "billing", agent, "", "", "Do the work.")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("move by a billing author = %d: %s; want 404", rec.Code, rec.Body)
	}
}

type etagJourney struct{ handler http.Handler }

func etagRoute(t *testing.T) etagJourney {
	t.Helper()
	pool := testdb.Pool(t)
	api := httpapi.NewServer(ledger.NewMemory(), "test").
		WithAgents(spec.NewRegistry(pool)).
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{})).
		WithRetirements(spec.NewState(pool))
	return etagJourney{handler: apiHandlerFor(api)}
}

func areaAuthor(area string) domain.Principal {
	return domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: domain.AreaID(area)}, Role: domain.RoleAuthor}},
	}
}

func (r etagJourney) put(t *testing.T, caller domain.Principal, agent, ifMatch, ifNoneMatch, text string) *httptest.ResponseRecorder {
	t.Helper()
	return r.putIn(t, caller, "factory", agent, ifMatch, ifNoneMatch, text)
}

func (r etagJourney) putIn(
	t *testing.T, caller domain.Principal, area, agent, ifMatch, ifNoneMatch, text string,
) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(openapi.AgentDefinition{
		Name: agent, Company: "acme", Area: area,
		Provider: "openai", Model: "gpt-test", Instructions: text,
		Tools:  &[]string{"crm.lookup"},
		Budget: &openapi.Budget{Micros: ptrTo(int64(500_000)), Steps: ptrTo(int64(60))},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	headers := map[string]string{"If-Match": ifMatch, "If-None-Match": ifNoneMatch}
	return r.do(t, caller, http.MethodPut, "/api/v1/agents/"+agent+"/versions", body, headers)
}

func (r etagJourney) retire(t *testing.T, caller domain.Principal, agent, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	return r.do(t, caller, http.MethodPut, "/api/v1/agents/"+agent+"/retired",
		[]byte(`{"retired":true}`), map[string]string{"If-Match": ifMatch})
}

func (r etagJourney) tag(t *testing.T, caller domain.Principal, agent string) string {
	t.Helper()
	rec := r.do(t, caller, http.MethodGet, "/api/v1/agents/"+agent, nil, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") == "" {
		t.Fatalf("GET %s = %d (ETag %q): %s", agent, rec.Code, rec.Header().Get("ETag"), rec.Body)
	}
	return rec.Header().Get("ETag")
}

func (r etagJourney) do(
	t *testing.T, caller domain.Principal, method, path string, body []byte, headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		if value != "" {
			req.Header.Set(name, value)
		}
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), caller))
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}
