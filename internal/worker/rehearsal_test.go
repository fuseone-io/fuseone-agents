package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
The rehearsal mark reaches the engine from the run's opening (NT-018 phase 4), and only from
there. A rehearsal whose worker crashed after recording a call resumes by waiting for its
author; a battery in the same place records the unknown outcome it always has.
*/
func TestTurn_theRehearsalMark_reachesTheEngineFromTheOpening(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		started domain.RunStartedPayload
		want    domain.StepKind
	}{
		"a rehearsal": {domain.RunStartedPayload{
			Trigger: "rehearsal", Simulated: true, Simulation: "reh-1", Rehearsal: true, SealID: testSeal}, domain.StepWorldAwaited},
		"a battery": {domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "sim-1", Case: "case-1"}, domain.StepBudgetReconciled},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			openSimulated(t, store, tc.started)
			for _, s := range []domain.Step{
				{Kind: domain.StepBudgetReserved, Payload: mustPayload(t, domain.BudgetReservedPayload{Micros: 1000})},
				{Kind: domain.StepToolCalled, Payload: mustPayload(t, domain.ToolCalledPayload{Tool: "crm.lookup", Effect: domain.EffectRead})},
			} {
				s.RunID, s.Scope, s.At = "run-1", domain.Scope{Company: "acme", Area: "cx"}, time.Now()
				if _, err := store.Append(context.Background(), s); err != nil {
					t.Fatalf("seed %s: %v", s.Kind, err)
				}
			}
			specs := &experimentSpecs{planner: &flakyPlanner{}}

			candidateTurn(t, store, store.Simulations(), specs, &recordingCandidates{planner: &flakyPlanner{}})

			head, err := store.Head(context.Background(), "run-1")
			if err != nil || head.Kind != tc.want {
				t.Errorf("head = %s (%v), want %s", head.Kind, err, tc.want)
			}
		})
	}
}

func mustPayload(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
