package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
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
Publishing the version that closes an event cycle answers with a warning
naming the cycle (#227), from the route the Studio calls.

Through the router, the JSON on the wire and the Postgres publisher serve
wires, which judges the graph inside its own transaction, because the warning
crosses every one of them: a field the contract dropped, a graph read from
somewhere else, or a publisher that never asked would each publish silently,
and a test calling the handler with a fake would pass all three.
*/
func TestPublishRoute_theVersionThatClosesACycle_publishesAndNamesIt(t *testing.T) {
	route := cycleRoute(t)
	// The database outlives the run; names of its own keep an earlier run's
	// agents out of this one's graph.
	run := fmt.Sprintf("i227-%d", time.Now().UnixNano())
	developer, reviewer := run+"-developer", run+"-reviewer"
	ready, done := run+".code.ready", run+".review.done"

	first := route.publish(t, developer, ready, done)
	if first.EventCycleWarning != nil {
		t.Fatalf("the first half warned %+v; nothing listens to %s yet", first.EventCycleWarning, ready)
	}

	second := route.publish(t, reviewer, done, ready)

	want := &openapi.EventCycleWarning{Cycles: [][]string{{reviewer, developer}}, HopLimit: 8}
	if !reflect.DeepEqual(second.EventCycleWarning, want) {
		t.Fatalf("warning = %+v, want %+v", second.EventCycleWarning, want)
	}
	if !second.Created {
		t.Error("the version closing the cycle was not written; a cycle is a warning, not a refusal")
	}
}

/*
Across areas, the real route names only what its caller may read (review
round 2).

The registry is what tells the answer where each agent lives. An author
granted acme/factory alone closes a loop with an agent in acme/billing: the
loop is reported, and the billing agent's name is not. Through the real
publisher, so a wiring read that dropped the scope — which would make every
agent look like it lives nowhere — is caught here and not only in a fake.
*/
func TestPublishRoute_aCycleIntoAnAreaTheCallerCannotRead_hidesThatAgent(t *testing.T) {
	route := cycleRoute(t)
	run := fmt.Sprintf("i227-%d", time.Now().UnixNano())
	invoicer, developer := run+"-invoicer", run+"-developer"
	ready, done := run+".code.ready", run+".review.done"
	route.publishIn(t, openPrincipal(), "billing", invoicer, done, ready)

	got := route.publishIn(t, factoryAuthor(), "factory", developer, ready, done)

	want := &openapi.EventCycleWarning{Cycles: [][]string{{developer, ""}}, HopLimit: 8}
	if !reflect.DeepEqual(got.EventCycleWarning, want) {
		t.Fatalf("warning = %+v, want %+v", got.EventCycleWarning, want)
	}
}

// A caller who may read the whole company reads the billing agent's name: the
// scope the registry reads is the agent's own, not one that hides everybody.
func TestPublishRoute_aCycleAcrossAreasOfACompanyTheCallerReads_namesEveryAgent(t *testing.T) {
	route := cycleRoute(t)
	run := fmt.Sprintf("i227-%d", time.Now().UnixNano())
	invoicer, developer := run+"-invoicer", run+"-developer"
	ready, done := run+".code.ready", run+".review.done"
	route.publishIn(t, openPrincipal(), "billing", invoicer, done, ready)

	got := route.publishIn(t, acmeAuthor(), "factory", developer, ready, done)

	want := &openapi.EventCycleWarning{Cycles: [][]string{{developer, invoicer}}, HopLimit: 8}
	if !reflect.DeepEqual(got.EventCycleWarning, want) {
		t.Fatalf("warning = %+v, want %+v", got.EventCycleWarning, want)
	}
}

// acmeAuthor may author agents anywhere in acme.
func acmeAuthor() domain.Principal {
	return domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme"}, Role: domain.RoleAuthor}},
	}
}

// factoryAuthor may author agents in acme/factory and nowhere else.
func factoryAuthor() domain.Principal {
	return domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "factory"}, Role: domain.RoleAuthor}},
	}
}

type publishRoute struct{ handler http.Handler }

func cycleRoute(t *testing.T) publishRoute {
	t.Helper()
	pool := testdb.Pool(t)
	registry := spec.NewRegistry(pool)
	api := httpapi.NewServer(ledger.NewMemory(), "test").
		WithAgents(registry).
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{}))
	return publishRoute{handler: apiHandlerFor(api)}
}

// publish is the Studio's PUT for an agent that emits one event and starts on
// another, and the answer it reads, by the open installation's administrator.
func (r publishRoute) publish(t *testing.T, agent, emits, listens string) openapi.AgentPublished {
	t.Helper()
	return r.publishIn(t, openPrincipal(), "factory", agent, emits, listens)
}

// publishIn is publish by a caller, into an area of acme.
func (r publishRoute) publishIn(
	t *testing.T, caller domain.Principal, area, agent string, emits, listens string,
) openapi.AgentPublished {
	t.Helper()
	body, err := json.Marshal(openapi.AgentDefinition{
		Name: agent, Company: "acme", Area: area,
		Provider: "openai", Model: "gpt-test", Instructions: "Do the work.",
		Tools:    &[]string{"crm.lookup"},
		Budget:   &openapi.Budget{Micros: ptrTo(int64(500_000)), Steps: ptrTo(int64(60))},
		Emits:    &[]openapi.AgentEvent{{Event: emits}},
		Triggers: &[]openapi.AgentTrigger{{Type: "event", Event: &listens}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		"/api/v1/agents/"+agent+"/versions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), caller))
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT %s = %d: %s", agent, rec.Code, rec.Body)
	}
	var out openapi.AgentPublished
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}
