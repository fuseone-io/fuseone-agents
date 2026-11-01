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

Through the router, the JSON on the wire and the registry serve wires as the
composition, because the warning crosses every one of them: a field the
contract dropped, a graph read from somewhere else, or a server that never
asked would each publish silently, and a test calling the handler with a fake
graph would pass all three.
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

type publishRoute struct{ handler http.Handler }

func cycleRoute(t *testing.T) publishRoute {
	t.Helper()
	pool := testdb.Pool(t)
	registry := spec.NewRegistry(pool)
	api := httpapi.NewServer(ledger.NewMemory(), "test").
		WithAgents(registry).
		WithComposition(registry).
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{}))
	return publishRoute{handler: openAccess(apiHandlerFor(api))}
}

// publish is the Studio's PUT for an agent that emits one event and starts on
// another, and the answer it reads.
func (r publishRoute) publish(t *testing.T, agent, emits, listens string) openapi.AgentPublished {
	t.Helper()
	body, err := json.Marshal(openapi.AgentDefinition{
		Name: agent, Company: "acme", Area: "factory",
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
