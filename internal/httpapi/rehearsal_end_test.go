package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
An abandoned rehearsal never ends holding a reservation (NT-018 phase 4). The planner
reserves nothing for a tool call today, so a rehearsal driven end to end waits with nothing
reserved; this one is seeded with what a call that estimated its cost would leave, waiting
for the world with that budget still held.
*/

func waitingWithAReservation(t *testing.T) *ledger.Memory {
	t.Helper()
	store := ledger.NewMemory()
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for i, step := range []domain.Step{
		{Kind: domain.StepRunStarted, Payload: mustPayload(t, domain.RunStartedPayload{Simulated: true, Rehearsal: true, Simulation: "reh"})},
		{Kind: domain.StepBudgetReserved, Payload: mustPayload(t, domain.BudgetReservedPayload{Micros: 700, Tokens: 30})},
		{Kind: domain.StepToolCalled, Payload: mustPayload(t, domain.ToolCalledPayload{Tool: "crm.lookup", Effect: domain.EffectRead})},
		{Kind: domain.StepWorldAwaited, Payload: mustPayload(t, domain.WorldAwaitedPayload{Tool: "crm.lookup", CallSeq: 3})},
	} {
		step.RunID, step.Scope, step.AgentID, step.VersionID = "run-reh", domain.Scope{Company: "acme", Area: "cx"}, "triage", "v1"
		step.At = at.Add(time.Duration(i) * time.Second)
		if _, err := store.Append(context.Background(), step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}
	return store
}

func TestAbandonRehearsal_waitingWithBudgetReserved_releasesItBeforeEnding(t *testing.T) {
	t.Parallel()
	store := waitingWithAReservation(t)

	resp, err := NewServer(store, "test").AbandonRehearsal(inArea("cx", domain.RoleAuthor),
		openapi.AbandonRehearsalRequestObject{RunId: "run-reh"})
	if _, ok := resp.(openapi.AbandonRehearsal200JSONResponse); err != nil || !ok {
		t.Fatalf("AbandonRehearsal = %T (%v)", resp, err)
	}
	steps, err := store.Read(context.Background(), "run-reh", domain.FirstSeq)
	if err != nil {
		t.Fatal(err)
	}
	state, err := engine.Fold(steps)
	if err != nil {
		t.Fatal(err)
	}
	kinds := []domain.StepKind{steps[len(steps)-2].Kind, steps[len(steps)-1].Kind}
	if state.Reserved != (domain.Consumption{}) || state.Phase != engine.PhaseFailed ||
		kinds[0] != domain.StepBudgetReconciled || kinds[1] != domain.StepAbandoned {
		t.Errorf("reserved %+v, phase %v, ending %v; want the reservation released, then the rehearsal ended", state.Reserved, state.Phase, kinds)
	}
}

// waitingWithNothingReserved is the same wait with its reservation already released.
func waitingWithNothingReserved(t *testing.T) *ledger.Memory {
	t.Helper()
	store := waitingWithAReservation(t)
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: "run-reh", Kind: domain.StepBudgetReconciled, Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: "v1", At: time.Date(2026, 11, 20, 12, 0, 9, 0, time.UTC),
		Payload: mustPayload(t, domain.BudgetReconciledPayload{ReleasedMicros: 700, ReleasedTokens: 30}),
	}); err != nil {
		t.Fatal(err)
	}
	return store
}

// With nothing reserved there is nothing to release: the ending is the only step.
func TestAbandonRehearsal_withNothingReserved_writesOnlyItsEnding(t *testing.T) {
	t.Parallel()
	store := waitingWithNothingReserved(t)
	before, _ := store.Read(context.Background(), "run-reh", domain.FirstSeq)

	if _, err := NewServer(store, "test").AbandonRehearsal(inArea("cx", domain.RoleAuthor),
		openapi.AbandonRehearsalRequestObject{RunId: "run-reh"}); err != nil {
		t.Fatal(err)
	}
	after, _ := store.Read(context.Background(), "run-reh", domain.FirstSeq)
	if len(after) != len(before)+1 || after[len(after)-1].Kind != domain.StepAbandoned {
		t.Errorf("wrote %d steps ending in %s, want only the ending", len(after)-len(before), after[len(after)-1].Kind)
	}
}

// movingStore ends the run just before the first write a route makes, as another tab or
// another author would between the route's read and its append.
type movingStore struct {
	*ledger.Memory
	t     *testing.T
	moved bool
}

func (m *movingStore) move(ctx context.Context) {
	if m.moved {
		return
	}
	m.moved = true
	if _, err := m.Memory.Append(ctx, domain.Step{
		RunID: "run-reh", Kind: domain.StepAbandoned, Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: "v1", At: time.Date(2026, 11, 20, 12, 0, 30, 0, time.UTC),
		Payload: mustPayload(m.t, domain.AbandonedPayload{By: "usr_bia", Reason: "elsewhere"}),
	}); err != nil {
		m.t.Fatalf("move the run: %v", err)
	}
}

func (m *movingStore) Append(ctx context.Context, s domain.Step) (domain.Step, error) {
	m.move(ctx)
	return m.Memory.Append(ctx, s)
}

func (m *movingStore) AppendIfHead(ctx context.Context, head domain.StepRef, s domain.Step) (domain.Step, error) {
	m.move(ctx)
	return m.Memory.AppendIfHead(ctx, head, s)
}

func heldForADecision(t *testing.T) *ledger.Memory {
	t.Helper()
	store := ledger.NewMemory()
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for i, step := range []domain.Step{
		{Kind: domain.StepRunStarted, Payload: mustPayload(t, domain.RunStartedPayload{Simulated: true, Rehearsal: true, Simulation: "reh"})},
		{Kind: domain.StepApprovalRequested, Payload: mustPayload(t, domain.ApprovalRequestedPayload{Tool: "crm.note", Rule: "policy"})},
	} {
		step.RunID, step.Scope, step.AgentID, step.VersionID = "run-reh", domain.Scope{Company: "acme", Area: "cx"}, "triage", "v1"
		step.At = at.Add(time.Duration(i) * time.Second)
		if _, err := store.Append(context.Background(), step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}
	return store
}

// Each acting route writes only onto the step it judged: a rehearsal that moved between
// the route's read and its write is told so, and nothing lands after the move.
func TestRehearsalActs_aRunThatMovedBeforeTheWrite_isToldAndWritesNothing(t *testing.T) {
	t.Parallel()
	ctx := inArea("cx", domain.RoleAuthor)
	cases := map[string]struct {
		seed func(*testing.T) *ledger.Memory
		act  func(*Server) (any, error)
	}{
		"an answer": {waitingWithAReservation, func(s *Server) (any, error) {
			return s.AnswerForTheWorld(ctx, openapi.AnswerForTheWorldRequestObject{RunId: "run-reh",
				Body: &openapi.AnswerForTheWorldJSONRequestBody{AtSeq: 4, Answer: "{}"}})
		}},
		"a decision": {heldForADecision, func(s *Server) (any, error) {
			return s.DecideInRehearsal(ctx, openapi.DecideInRehearsalRequestObject{RunId: "run-reh",
				Body: &openapi.DecideInRehearsalJSONRequestBody{AtSeq: 2, Approved: true}})
		}},
		"an ending that releases first": {waitingWithAReservation, func(s *Server) (any, error) {
			return s.AbandonRehearsal(ctx, openapi.AbandonRehearsalRequestObject{RunId: "run-reh"})
		}},
		"an ending with nothing to release": {waitingWithNothingReserved, func(s *Server) (any, error) {
			return s.AbandonRehearsal(ctx, openapi.AbandonRehearsalRequestObject{RunId: "run-reh"})
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := &movingStore{Memory: c.seed(t), t: t}
			resp, err := c.act(NewServer(store, "test").WithContent(engine.NewMemoryContent()))
			if err != nil {
				t.Fatalf("act: %v", err)
			}
			steps, _ := store.Read(context.Background(), "run-reh", domain.FirstSeq)
			if last := steps[len(steps)-1]; last.Kind != domain.StepAbandoned || !store.moved {
				t.Errorf("after the move the run ends in %s (%T): a write landed after it", last.Kind, resp)
			}
			if !isConflictAnswer(resp) {
				t.Errorf("answer = %T, want a 409", resp)
			}
		})
	}
}

func isConflictAnswer(resp any) bool {
	switch resp.(type) {
	case openapi.AnswerForTheWorld409ApplicationProblemPlusJSONResponse,
		openapi.DecideInRehearsal409ApplicationProblemPlusJSONResponse,
		openapi.AbandonRehearsal409ApplicationProblemPlusJSONResponse:
		return true
	}
	return false
}
