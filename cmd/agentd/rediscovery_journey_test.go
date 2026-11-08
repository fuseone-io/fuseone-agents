package main

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A rediscovery asked for in the administration reaches the worker that holds the
server, through the store the worker is really given (#287). The unit tests
hand the reconciler a fake that answers the store's questions; this one hands
it the installation's own integrations, so a store that stopped satisfying the
worker's interface would show here as a request nobody answered, not as a
silent type assertion.
*/
func TestRediscovery_requestedInTheAdministration_isAnsweredByEachWorkerThatHoldsTheServer(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	if _, err := j.pool.Exec(t.Context(), `truncate mcp_rediscovery_receipts, mcp_rediscoveries`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	j.worker.holder = "worker-a"
	j.discover(t)

	generation, err := j.integrations.RequestMCPRediscovery(t.Context(), "usr_ana",
		domain.Scope{Company: "default", Area: "platform"}, j.server)
	if err != nil {
		t.Fatalf("RequestMCPRediscovery: %v", err)
	}
	j.discover(t)
	late := j.replica(t)
	late.holder = "worker-b"
	late.reconcile(t.Context())

	status, err := j.integrations.MCPRediscovery(t.Context(), j.server)
	if err != nil {
		t.Fatalf("MCPRediscovery: %v", err)
	}
	answered := map[string]int64{}
	for _, r := range status.Receipts {
		if r.Succeeded {
			answered[r.Worker] = r.Generation
		}
	}
	if answered["worker-a"] != generation || answered["worker-b"] != generation {
		t.Errorf("answered = %v, want both workers at generation %d", answered, generation)
	}
	if !j.catalog.Connected(j.server) {
		t.Error("the worker that rediscovered lost its session")
	}
}
