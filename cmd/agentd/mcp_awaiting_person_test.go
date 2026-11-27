package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A server only a person can discover, with nobody connected, waits for a
// person (#483, delivery 3b). The worker says so in what it observed, under the
// configuration it observed it with: a run is refused on it, and the console
// shows the wait, only while that is still the configuration.

var dpgGeneration = time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)

func TestReconcile_nobodyConnected_recordsTheWaitUnderTheServersGeneration(t *testing.T) {
	t.Parallel()
	_, url := newIdentityServer(t)
	w := withDiscovery(t, url)
	w.store.update(func(s *discoveryStore) { s.server.UpdatedAt, s.discoverer = dpgGeneration, "" })

	w.r.reconcile(t.Context())

	h := w.health["dpg"]
	if h.Reachable || !h.AwaitingPerson || !h.AwaitingGeneration.Equal(dpgGeneration) {
		t.Errorf("health = %+v, want unreachable and awaiting a person under %s", h, dpgGeneration)
	}
}

func TestReconcile_theLastPersonLeaving_recordsTheWait(t *testing.T) {
	t.Parallel()
	_, url := newIdentityServer(t)
	w := withDiscovery(t, url)
	w.store.update(func(s *discoveryStore) { s.server.UpdatedAt = dpgGeneration })
	w.r.reconcile(t.Context())

	w.store.update(func(s *discoveryStore) { s.creds, s.discoverer = personalCreds{}, "" })
	w.r.reconcile(t.Context())

	if h := w.health["dpg"]; !h.AwaitingPerson || !h.AwaitingGeneration.Equal(dpgGeneration) {
		t.Errorf("health = %+v, want the wait once the last person left", h)
	}
}

// Every other failure is what it is: the store failing to say, or the server
// down, is never a server waiting for a person.
func TestReconcile_anyOtherFailure_isNotAWaitForAPerson(t *testing.T) {
	t.Parallel()
	failing := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		http.Error(rw, "down", http.StatusBadGateway)
	}))
	t.Cleanup(failing.Close)
	_, refusing := newIdentityServer(t)

	cases := map[string]struct {
		url   string
		store func(*discoveryStore)
	}{
		"the store failing to say": {refusing, func(s *discoveryStore) {
			s.discoverer, s.discovererErr = "", errors.New("settings unreachable")
		}},
		"the server down": {failing.URL, func(s *discoveryStore) { s.discoverer = "" }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := withDiscovery(t, c.url)
			w.store.update(c.store)

			w.r.reconcile(t.Context())

			if h := w.health["dpg"]; h.Reachable || h.AwaitingPerson {
				t.Errorf("health = %+v, want a failure that is not a wait for a person", h)
			}
		})
	}
}
