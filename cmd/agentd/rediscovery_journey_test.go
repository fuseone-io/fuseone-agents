package main

import (
	"testing"

	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
A rediscovery asked for in the administration reaches the worker that holds the
server, through the store the worker is really given (#287). The unit tests
hand the reconciler a fake that answers the store's questions; this one hands
it the installation's own integrations behind the real route, so a store that stopped satisfying the
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

	resp, err := j.api.RequestMCPRediscovery(installer(t),
		openapi.RequestMCPRediscoveryRequestObject{Name: j.server})
	accepted, ok := resp.(openapi.RequestMCPRediscovery202JSONResponse)
	if err != nil || !ok {
		t.Fatalf("RequestMCPRediscovery = %#v, %v; want 202", resp, err)
	}
	generation := accepted.Generation
	j.discover(t)
	late := j.replica(t)
	late.holder = "worker-b"
	late.reconcile(t.Context())

	read, err := j.api.GetMCPRediscovery(installer(t), openapi.GetMCPRediscoveryRequestObject{Name: j.server})
	status, ok := read.(openapi.GetMCPRediscovery200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetMCPRediscovery = %#v, %v; want 200", read, err)
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
