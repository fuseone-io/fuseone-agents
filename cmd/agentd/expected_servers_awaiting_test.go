package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// observedHealth is what the worker last recorded, by server.
type observedHealth map[string]domain.IntegrationHealth

func (o observedHealth) Of(_ context.Context, name string) (domain.IntegrationHealth, bool, error) {
	h, ok := o[name]
	return h, ok, nil
}

// whoDiscovers answers who connected last, counting asks.
type whoDiscovers struct {
	who  domain.UserID
	err  error
	asks int
}

func (w *whoDiscovers) MCPPersonalDiscoverer(context.Context, domain.Scope, string) (domain.UserID, bool, error) {
	w.asks++
	return w.who, w.who != "", w.err
}

var dpgServer = domain.MCPServer{Name: "dpg", Enabled: true, UpdatedAt: dpgGeneration}

func waitingFor(generation time.Time) observedHealth {
	return observedHealth{"dpg": {
		Name: "dpg", Reachable: false, AwaitingPerson: true, AwaitingGeneration: generation,
	}}
}

// A run is refused for good only on what is still true: the wait observed
// under the configuration as it is now, and nobody connected when asked again.
func TestAwaitingPerson_theWaitUnderTheCurrentConfigurationWithNobodyConnected_refuses(t *testing.T) {
	t.Parallel()
	who := &whoDiscovers{}
	e := expectedServers{servers: configuredServers{dpgServer}, health: waitingFor(dpgGeneration), discoverers: who}

	waiting, err := e.AwaitingPerson(t.Context(), "dpg")

	if err != nil || !waiting {
		t.Fatalf("AwaitingPerson = %v, %v, want waiting", waiting, err)
	}
	if who.asks != 1 {
		t.Errorf("asked who discovers %d times, want the store asked again before refusing", who.asks)
	}
}

func TestAwaitingPerson_anythingNoLongerTrue_isNotAWait(t *testing.T) {
	t.Parallel()
	reachable := waitingFor(dpgGeneration)
	h := reachable["dpg"]
	h.Reachable = true
	reachable["dpg"] = h

	cases := map[string]struct {
		servers configuredServers
		health  observedHealth
		who     domain.UserID
	}{
		"a person connected since":            {configuredServers{dpgServer}, waitingFor(dpgGeneration), "usr_ana"},
		"the configuration changed since":     {configuredServers{dpgServer}, waitingFor(dpgGeneration.Add(-time.Second)), ""},
		"reachable since, by an older binary": {configuredServers{dpgServer}, reachable, ""},
		"never observed waiting":              {configuredServers{dpgServer}, observedHealth{}, ""},
		"the server disabled":                 {configuredServers{{Name: "dpg", UpdatedAt: dpgGeneration}}, waitingFor(dpgGeneration), ""},
		"the server removed":                  {configuredServers{}, waitingFor(dpgGeneration), ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := expectedServers{servers: c.servers, health: c.health, discoverers: &whoDiscovers{who: c.who}}
			waiting, err := e.AwaitingPerson(t.Context(), "dpg")
			if err != nil || waiting {
				t.Errorf("AwaitingPerson = %v, %v, want no wait", waiting, err)
			}
		})
	}
}

// The store failing to say who connected is not nobody: the turn fails with
// the read, and is supervised like any other fault.
func TestAwaitingPerson_theStoreFailingToSay_isAnError(t *testing.T) {
	t.Parallel()
	e := expectedServers{
		servers: configuredServers{dpgServer}, health: waitingFor(dpgGeneration),
		discoverers: &whoDiscovers{err: errors.New("settings unreachable")},
	}

	if waiting, err := e.AwaitingPerson(t.Context(), "dpg"); err == nil || waiting {
		t.Errorf("AwaitingPerson = %v, %v, want the read's failure", waiting, err)
	}
}

// A worker with nowhere to read what was observed, or who connected, never
// refuses a run for good.
func TestAwaitingPerson_withoutTheReaders_isNeverAWait(t *testing.T) {
	t.Parallel()
	for name, e := range map[string]expectedServers{
		"no health":      {servers: configuredServers{dpgServer}, discoverers: &whoDiscovers{}},
		"no discoverers": {servers: configuredServers{dpgServer}, health: waitingFor(dpgGeneration)},
	} {
		if waiting, err := e.AwaitingPerson(t.Context(), "dpg"); err != nil || waiting {
			t.Errorf("%s: AwaitingPerson = %v, %v, want no wait", name, waiting, err)
		}
	}
}
