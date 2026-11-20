package engine

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
The reading side of a live rehearsal (NT-018 phase 4, release 4a). Nothing writes these
steps yet; this release must read whatever a later one writes, since it becomes the
rollback floor: a run waiting for its author stays waiting, an answer becomes the call's
result, a crash on either side resumes in the rehearsal's own way, and the author's
decision answers the request the Gate made.
*/

// seed appends hand-built steps to the harness's ledger for start's run.
func (h *harness) seed(t *testing.T, start Start, steps ...domain.Step) {
	t.Helper()
	for _, s := range steps {
		s.RunID, s.Scope = start.RunID, start.Scope
		s.At = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
		if _, err := h.ledger.Append(context.Background(), s); err != nil {
			t.Fatalf("seed Append(%s): %v", s.Kind, err)
		}
	}
}

// calledOnce is a rehearsal's opening, a reservation and one recorded call, at seqs 1-3.
func calledOnce(start Start) []domain.Step {
	return []domain.Step{
		{Kind: domain.StepRunStarted, Payload: mustJSON(domain.RunStartedPayload{Simulated: true, Rehearsal: true})},
		{Kind: domain.StepBudgetReserved, Payload: mustJSON(domain.BudgetReservedPayload{Micros: 30_000})},
		{Kind: domain.StepToolCalled, IdemKey: idempotencyKey(start.RunID, "crm.lookup", []byte(`{}`)),
			Payload: mustJSON(domain.ToolCalledPayload{Tool: "crm.lookup", Effect: domain.EffectRead})},
	}
}

func rehearsalStart(t *testing.T, h *harness) Start {
	t.Helper()
	start := h.start(t, generousBudget())
	start.Rehearsal = true
	return start
}

func TestAdvance_aRehearsalWaitingForTheWorld_staysWaitingAndCallsNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{}`)})
	start := rehearsalStart(t, h)
	h.seed(t, start, calledOnce(start)...)
	h.seed(t, start, domain.Step{Kind: domain.StepWorldAwaited,
		Payload: mustJSON(domain.WorldAwaitedPayload{Tool: "crm.lookup", CallSeq: 3})})

	st, err := h.runner.Advance(context.Background(), start)
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	steps, _ := h.ledger.Read(context.Background(), start.RunID, domain.FirstSeq)
	if st.Phase != PhaseAwaitingWorld || len(steps) != 4 || len(h.tools.invocations) != 0 {
		t.Errorf("phase %v, %d steps, %d calls; want waiting, untouched, nothing called", st.Phase, len(steps), len(h.tools.invocations))
	}
}

// The author's answer is the call's result: untrusted, named back to the call it answers,
// and the reservation released, all without reaching any tool.
func TestAdvance_theAuthorsAnswer_becomesTheCallsResult(t *testing.T) {
	t.Parallel()
	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{}`)})
	start := rehearsalStart(t, h)
	h.seed(t, start, calledOnce(start)...)
	h.seed(t, start,
		domain.Step{Kind: domain.StepWorldAwaited, Payload: mustJSON(domain.WorldAwaitedPayload{Tool: "crm.lookup", CallSeq: 3})},
		domain.Step{Kind: domain.StepWorldSupplied, Payload: mustJSON(domain.WorldSuppliedPayload{
			AtSeq: 4, Ref: "run://run-1/5/abc", Digest: "abc", Bytes: 12, By: "usr_ana"})})

	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	steps, _ := h.ledger.Read(context.Background(), start.RunID, domain.FirstSeq)
	returned := steps[5]
	var p domain.ToolReturnedPayload
	if err := decode(returned, &p); err != nil || returned.Kind != domain.StepToolReturned {
		t.Fatalf("step 6 = %s (%v), want the call's result", returned.Kind, err)
	}
	if p.ForSeq != 3 || p.ResultRef != "run://run-1/5/abc" || p.ResultDigest != "abc" || !returned.Labels.HasAny(domain.LabelUntrusted) {
		t.Errorf("result = %+v labels %v, want the answer, for call 3, untrusted", p, returned.Labels)
	}
	state, _ := Fold(steps)
	if state.Reserved.Micros != 0 || state.Phase != PhaseRunning || len(h.tools.invocations) != 0 {
		t.Errorf("reserved %d, phase %v, %d calls; want released, running, nothing called", state.Reserved.Micros, state.Phase, len(h.tools.invocations))
	}
}

// A worker that died after recording a rehearsal's call resumes by waiting for the author,
// not by recording an unknown outcome: nothing was ever going to reach the world.
func TestAdvance_aRehearsalCallLeftBehindByACrash_waitsForTheAuthor(t *testing.T) {
	t.Parallel()
	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{}`)})
	start := rehearsalStart(t, h)
	h.seed(t, start, calledOnce(start)...)

	st, err := h.runner.Advance(context.Background(), start)
	if err != nil {
		t.Fatalf("Advance: %v", err)
	}
	var p domain.WorldAwaitedPayload
	if err := h.lastPayloadOf(t, domain.StepWorldAwaited, &p); err != nil || p.CallSeq != 3 || p.Tool != "crm.lookup" {
		t.Fatalf("world_awaited = %+v (%v), want the wait for call 3", p, err)
	}
	if st.Phase != PhaseAwaitingWorld || len(h.tools.invocations) != 0 {
		t.Errorf("phase %v, %d calls; want waiting, nothing called", st.Phase, len(h.tools.invocations))
	}
}

// An answer that names another wait is not this call's result.
func TestFold_anAnswerToAnotherWait_isNotTaken(t *testing.T) {
	t.Parallel()
	h := newHarness(t, Proposal{Tool: "crm.lookup", Args: []byte(`{}`)})
	start := rehearsalStart(t, h)
	h.seed(t, start, calledOnce(start)...)
	h.seed(t, start,
		domain.Step{Kind: domain.StepWorldAwaited, Payload: mustJSON(domain.WorldAwaitedPayload{Tool: "crm.lookup", CallSeq: 3})},
		domain.Step{Kind: domain.StepWorldSupplied, Payload: mustJSON(domain.WorldSuppliedPayload{
			AtSeq: 2, Ref: "run://run-1/5/abc", Digest: "abc", By: "usr_ana"})})

	steps, _ := h.ledger.Read(context.Background(), start.RunID, domain.FirstSeq)
	state, err := Fold(steps)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	if state.Phase != PhaseAwaitingWorld || state.supplied != nil {
		t.Errorf("phase %v, supplied %+v; want still waiting, nothing taken", state.Phase, state.supplied)
	}
}

// The author's decision answers the Gate's request as a person's would, and only that.
func TestFold_theAuthorsDecision_answersTheRequestTheGateMade(t *testing.T) {
	t.Parallel()
	for name, approved := range map[string]bool{"approved": true, "refused": false} {
		h := newHarness(t, Proposal{Tool: "crm.refund", Args: []byte(`{}`)})
		start := rehearsalStart(t, h)
		h.seed(t, start,
			domain.Step{Kind: domain.StepRunStarted, Payload: mustJSON(domain.RunStartedPayload{Simulated: true, Rehearsal: true})},
			domain.Step{Kind: domain.StepApprovalRequested, Payload: mustJSON(domain.ApprovalRequestedPayload{Tool: "crm.refund"})},
			domain.Step{Kind: domain.StepRehearsalDecided, Payload: mustJSON(domain.RehearsalDecidedPayload{AtSeq: 2, Approved: approved, By: "usr_ana"})})

		steps, _ := h.ledger.Read(context.Background(), start.RunID, domain.FirstSeq)
		state, err := Fold(steps)
		if err != nil {
			t.Fatalf("%s: Fold: %v", name, err)
		}
		if state.Phase != PhaseRunning || state.PendingApproval != nil || (state.Approved != nil) != approved {
			t.Errorf("%s: phase %v, pending %+v, approved %+v", name, state.Phase, state.PendingApproval, state.Approved)
		}
	}
}

func TestPhase_awaitingWorld_isNeitherTerminalNorResumable(t *testing.T) {
	t.Parallel()
	s := State{Phase: PhaseAwaitingWorld}
	if s.Terminal() || s.Resumable() || PhaseAwaitingWorld.String() != "awaiting_world" {
		t.Errorf("awaiting world: terminal %v, resumable %v, name %q", s.Terminal(), s.Resumable(), PhaseAwaitingWorld)
	}
}
