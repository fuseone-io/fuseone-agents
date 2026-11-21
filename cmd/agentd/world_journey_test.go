package main

import (
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
)

/*
The simulation pool the worker command builds reads a case's world from Postgres, at the
version its run was opened on (NT-018 phase 5). Both sides, from an experiment started at the
route: a world that reads is replayed and the run finishes, which a pool that reads no world
would refuse; a world erased since fails the run with fuseone:world-unreadable before anything
plans, rather than answer the case with nothing.
*/
func TestExperiment_aCaseWithAWorld_isReplayedByTheProductionPoolOrRefusedOnceErased(t *testing.T) {
	for name, tc := range map[string]struct {
		erase bool
		want  domain.StepKind
		code  string
	}{
		"a world that reads": {want: domain.StepRunFinished},
		"a world erased":     {erase: true, want: domain.StepFailed, code: "fuseone:world-unreadable"},
	} {
		t.Run(name, func(t *testing.T) {
			head := experimentOnAWorld(t, tc.erase)
			var failed domain.FailedPayload
			_ = json.Unmarshal(head.Payload, &failed)
			if head.Kind != tc.want || failed.Code != tc.code {
				t.Errorf("head %s %s, want %s %q", head.Kind, head.Payload, tc.want, tc.code)
			}
		})
	}
}

// experimentOnAWorld records the experiment's case with a world, erased or not, runs one
// experiment on it through the production pool, and answers where its run ended.
func experimentOnAWorld(t *testing.T, erase bool) domain.Step {
	t.Helper()
	world := newExperimentWorld(t)
	content := ledger.NewContent(world.pool)
	body, err := json.Marshal(domain.CaseWorld{Case: "refund"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	worldRef, err := content.PutFor(t.Context(), regression.WorldKind, string(world.agent)+"/refund", 1, body)
	if err != nil {
		t.Fatalf("keep the world: %v", err)
	}
	input, err := content.RestoreFor(t.Context(), "regression", string(world.agent), 1, []byte(`{"ask":"refund"}`))
	if err != nil {
		t.Fatalf("keep the occurrence: %v", err)
	}
	if err := regression.NewStore(world.pool).Record(t.Context(), domain.RegressionCase{
		ID: "refund", Agent: world.agent, Scope: domain.Scope{Company: "acme", Area: "cx"},
		InputRef: input, FromRun: "run-origin", World: worldRef,
		Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if erase {
		if _, err := world.pool.Exec(t.Context(), `
			update run_content set bytes = null, erased_at = now(), erased_reason = 'subject' where ref = $1`, worldRef); err != nil {
			t.Fatalf("erase the world: %v", err)
		}
	}

	resp, err := world.server.StartExperiment(author(t.Context()), openapi.StartExperimentRequestObject{
		AgentId: string(world.agent),
		Params:  openapi.StartExperimentParams{IdempotencyKey: "journey-world-1"},
		Body:    &openapi.ExperimentRequest{Repetitions: ptrTo(1)},
	})
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	accepted, ok := resp.(openapi.StartExperiment202JSONResponse)
	if !ok {
		t.Fatalf("StartExperiment answered %T %+v", resp, resp)
	}
	runs, err := world.store.SimulationRuns(t.Context(), accepted.Id)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %v err=%v", runs, err)
	}
	world.advanceUntil(t, runs[0], func(s engine.State) bool { return s.Phase == engine.PhaseFailed || s.Phase == engine.PhaseFinished })

	head, err := world.store.Head(t.Context(), runs[0])
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	return head
}
