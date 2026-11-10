package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
An experiment lives where its runs were recorded, not where its agent is published today.

An agent republished from finance to cx keeps its finance runs in finance. Somebody who may
read or publish in cx is allowed the agent as it is now, and has no claim on what was
recorded under finance — or on a version that was only ever published there. Every door
answers 404, as it would for an experiment that does not exist.
*/

func inFinance(v domain.AgentSummary) domain.AgentSummary {
	v.Scope = domain.Scope{Company: "acme", Area: "finance"}
	return v
}

// movedToCx is an agent first published in finance with one experiment there, then
// republished in cx.
func movedToCx(t *testing.T) (*Server, *ledger.Memory, *fakeDetail, string) {
	t.Helper()
	store := ledger.NewMemory()
	agents := &fakeDetail{versions: []domain.AgentSummary{inFinance(servedVersion("v1"))}}
	server, _ := experimentFixture(t, store, agents)
	resp, err := server.StartExperiment(inArea("finance", domain.RoleAuthor),
		startExperimentKeyed("exp-intent-0001", openapi.ExperimentRequest{}))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	accepted, ok := resp.(openapi.StartExperiment202JSONResponse)
	if !ok {
		t.Fatalf("response = %T %+v, want the finance experiment accepted", resp, resp)
	}
	agents.versions = []domain.AgentSummary{servedVersion("v2"), inFinance(servedVersion("v1"))}
	return server, store, agents, accepted.Id
}

func TestGetExperiment_recordedInAnotherScope_isNotFound(t *testing.T) {
	t.Parallel()
	server, _, _, id := movedToCx(t)

	resp, err := server.GetExperiment(inArea("cx", domain.RoleAuditor),
		openapi.GetExperimentRequestObject{AgentId: "triage", ExperimentId: id})
	if err != nil {
		t.Fatalf("GetExperiment: %v", err)
	}
	if _, ok := resp.(openapi.GetExperiment404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T, want the finance experiment hidden from cx", resp)
	}
}

func TestCompareExperiments_withOneRecordedInAnotherScope_isNotFound(t *testing.T) {
	t.Parallel()
	server, _, _, finance := movedToCx(t)
	cx := acceptKeyed(t, server, "exp-intent-0002", openapi.ExperimentRequest{})

	for _, params := range []openapi.CompareExperimentsParams{{From: finance, To: cx}, {From: cx, To: finance}} {
		resp, err := server.CompareExperiments(inArea("cx", domain.RoleAuditor),
			openapi.CompareExperimentsRequestObject{AgentId: "triage", Params: params})
		if err != nil {
			t.Fatalf("CompareExperiments: %v", err)
		}
		if _, ok := resp.(openapi.CompareExperiments404ApplicationProblemPlusJSONResponse); !ok {
			t.Errorf("CompareExperiments(%+v) = %T, want the finance side hidden from cx", params, resp)
		}
	}
}

func TestStartExperiment_aRetryOfOneRecordedInAnotherScope_isNotFoundAndAddsNothing(t *testing.T) {
	t.Parallel()
	server, store, _, id := movedToCx(t)
	before := len(startedRuns(t, store, id))

	resp, err := server.StartExperiment(inArea("cx", domain.RoleAuthor),
		startExperimentKeyed("exp-intent-0001", openapi.ExperimentRequest{}))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	if _, ok := resp.(openapi.StartExperiment404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T %+v, want the finance experiment hidden from cx", resp, resp)
	}
	if after := len(startedRuns(t, store, id)); after != before {
		t.Errorf("%d runs after the retry, want the %d finance opened", after, before)
	}
}

func TestStartExperiment_aVersionOnlyPublishedInAnotherScope_isNotFoundAndOpensNothing(t *testing.T) {
	t.Parallel()
	server, store, _, _ := movedToCx(t)
	id := experimentID("triage", "exp-intent-0003")

	resp, err := server.StartExperiment(inArea("cx", domain.RoleAuthor),
		startExperimentKeyed("exp-intent-0003", openapi.ExperimentRequest{Version: ptr("v1")}))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	if _, ok := resp.(openapi.StartExperiment404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T %+v, want the finance version hidden from cx", resp, resp)
	}
	if runs := startedRuns(t, store, id); len(runs) != 0 {
		t.Errorf("%d runs opened on the finance version", len(runs))
	}
}

// A retry that states another intention is still answered as if the experiment did not
// exist: a 409 would confirm it, and which field it names would let a caller probe what the
// finance experiment asked for. The scope is checked before the intentions are compared.
func TestStartExperiment_aDivergentRetryOfOneRecordedInAnotherScope_isNotFound(t *testing.T) {
	t.Parallel()
	server, store, _, id := movedToCx(t)
	before := len(startedRuns(t, store, id))

	resp, err := server.StartExperiment(inArea("cx", domain.RoleAuthor),
		startExperimentKeyed("exp-intent-0001", openapi.ExperimentRequest{Repetitions: ptr(3)}))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	if _, ok := resp.(openapi.StartExperiment404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T %+v, want the finance experiment hidden, not its divergence named", resp, resp)
	}
	if after := len(startedRuns(t, store, id)); after != before {
		t.Errorf("%d runs after the retry, want the %d finance opened", after, before)
	}
}
