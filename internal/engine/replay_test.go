package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

/*
A replayed case (NT-018 phase 5): the engine answers each call from the world the case was
recorded with, strictly in order and each entry once, and reaches no tool layer. The first
call that leaves the world is written down by the engine, once, as replay_diverged; every
call after it is answered with nothing. A run that finishes with entries left over diverged
by missing them. A worker that crashed after a replayed call resolves it again from the world:
the answer is pure, so there is no unknown outcome.
*/

func lookup(id string) Proposal {
	return Proposal{Tool: "crm.lookup", Args: []byte(`{"id":"` + id + `"}`)}
}

func recorded(answers ...string) domain.CaseWorld {
	w := domain.CaseWorld{Case: "reg-1"}
	for i, id := range answers {
		w.Entries = append(w.Entries, domain.WorldEntry{
			Position: i + 1, Tool: "crm.lookup", ArgsDigest: digest(lookup(id).Args),
			AnswerRef: "case_world://suporte/reg-1/" + id, AnswerDigest: "d-" + id, AnswerBytes: 7,
		})
	}
	return w
}

// kept stores each answer where the next turn's transcript reads it, as a case's copies are.
func kept(t *testing.T, h *harness, world domain.CaseWorld) *domain.CaseWorld {
	t.Helper()
	for i, e := range world.Entries {
		ref, err := h.content.Put(context.Background(), "case:reg-1", int64(e.Position), []byte(`{"answer":"`+e.AnswerDigest+`"}`))
		if err != nil {
			t.Fatalf("keep answer %d: %v", e.Position, err)
		}
		world.Entries[i].AnswerRef = ref
	}
	return &world
}

func replayHarness(t *testing.T, world domain.CaseWorld, proposals ...Proposal) (*harness, Start) {
	t.Helper()
	h := newHarness(t, append(proposals, Proposal{Done: true, Outcome: "done"})...)
	// A replayed call asks no tool layer anything, not even to get ready.
	h.tools.reserveErr = errors.New("a replayed call reached the tool layer")
	start := h.start(t, generousBudget())
	start.Pack = gate.NewPack("crm.lookup")
	start.World = kept(t, h, world)
	openRunWithInput(t, h, start, "simulation", []byte("the case"), domain.ScopeLabels(start.Scope))
	return h, start
}

// advanceToEnd advances the run until it rests, as a worker would, turn after turn.
func advanceToEnd(t *testing.T, h *harness, start Start) Status {
	t.Helper()
	for range 50 {
		st, err := h.runner.Advance(context.Background(), start)
		if err != nil {
			t.Fatalf("Advance: %v", err)
		}
		if st.Phase == PhaseFinished || st.Phase == PhaseFailed || st.Phase == PhaseAwaitingApproval {
			return st
		}
	}
	t.Fatal("the run never came to rest")
	return Status{}
}

func returned(t *testing.T, h *harness, start Start) []domain.ToolReturnedPayload {
	t.Helper()
	var out []domain.ToolReturnedPayload
	for _, s := range h.stepsOf(t, start.RunID, domain.StepToolReturned) {
		var p domain.ToolReturnedPayload
		if err := decode(s, &p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		out = append(out, p)
	}
	return out
}

func divergences(t *testing.T, h *harness, start Start) []domain.ReplayDivergedPayload {
	t.Helper()
	var out []domain.ReplayDivergedPayload
	for _, s := range h.stepsOf(t, start.RunID, domain.StepReplayDiverged) {
		var p domain.ReplayDivergedPayload
		if err := decode(s, &p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		out = append(out, p)
	}
	return out
}

func TestAdvance_aReplayedCaseFollowingItsWorld_getsEachAnswerInOrderAndReachesNoTool(t *testing.T) {
	t.Parallel()
	h, start := replayHarness(t, recorded("a", "b"), lookup("a"), lookup("b"))
	st := advanceToEnd(t, h, start)
	got := returned(t, h, start)
	if len(got) != 2 || got[0].ResultRef != start.World.Entries[0].AnswerRef || got[1].Replayed != 2 {
		t.Fatalf("results %+v, want the two recorded answers in order", got)
	}
	if d := divergences(t, h, start); len(d) != 0 || st.Phase != PhaseFinished || len(h.tools.invocations) != 0 {
		t.Errorf("divergences %+v, phase %v, %d calls; want none, finished, none", d, st.Phase, len(h.tools.invocations))
	}
}

// A recorded answer is what the author typed for the world: external content as far as the
// agent's next decision goes, exactly as it was in the rehearsal.
func TestAdvance_aReplayedAnswer_isUntrusted(t *testing.T) {
	t.Parallel()
	h, start := replayHarness(t, recorded("a"), lookup("a"))
	advanceToEnd(t, h, start)
	steps := h.stepsOf(t, start.RunID, domain.StepToolReturned)
	if len(steps) != 1 || !steps[0].Labels.Has(domain.LabelUntrusted) {
		t.Errorf("results %+v, want one labelled untrusted", steps)
	}
}

func TestAdvance_aReplayedCaseLeavingItsWorld_recordsTheFirstDivergenceOnce(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		world     domain.CaseWorld
		proposals []Proposal
		want      domain.ReplayDivergedPayload
	}{
		"extra":        {recorded("a"), []Proposal{lookup("z"), lookup("a")}, domain.ReplayDivergedPayload{Position: 1, Divergence: domain.DivergenceExtra, Tool: "crm.lookup"}},
		"out of order": {recorded("a", "b"), []Proposal{lookup("b"), lookup("a")}, domain.ReplayDivergedPayload{Position: 1, Divergence: domain.DivergenceOutOfOrder, Tool: "crm.lookup"}},
		"repeated":     {recorded("a", "b"), []Proposal{lookup("a"), lookup("a"), lookup("b")}, domain.ReplayDivergedPayload{Position: 2, Divergence: domain.DivergenceRepeated, Tool: "crm.lookup"}},
		"missing":      {recorded("a", "b"), []Proposal{lookup("a")}, domain.ReplayDivergedPayload{Position: 2, Divergence: domain.DivergenceMissing, Tool: "crm.lookup"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, start := replayHarness(t, tc.world, tc.proposals...)
			advanceToEnd(t, h, start)
			if got := divergences(t, h, start); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("divergences %+v, want exactly %+v", got, tc.want)
			}
			if head := headOf(t, h, start); head.Kind != domain.StepRunFinished {
				t.Errorf("head %s, want the run finished", head.Kind)
			}
		})
	}
}

// From the first divergence on, the run is answered with nothing: even a call that would
// match the next entry is no longer the situation the author corrected.
func TestAdvance_afterADivergence_everyCallIsAnsweredWithNothing(t *testing.T) {
	t.Parallel()
	h, start := replayHarness(t, recorded("a", "b"), lookup("z"), lookup("a"), lookup("b"))
	advanceToEnd(t, h, start)
	for i, p := range returned(t, h, start) {
		if p.ResultRef != "" || p.Replayed != 0 {
			t.Errorf("result %d = %+v, want nothing handed out", i+1, p)
		}
	}
}

// A worker that crashed after recording a replayed call resolves it again from the world on
// the next turn, the same way: no unknown outcome, because nothing outside the engine ran.
func TestAdvance_aReplayedCallACrashLeftBehind_isResolvedFromTheWorld(t *testing.T) {
	t.Parallel()
	h := newHarness(t, Proposal{Done: true, Outcome: "done"})
	start := h.start(t, generousBudget())
	start.World = kept(t, h, recorded("a"))
	world := *start.World
	h.seed(t, start,
		domain.Step{Kind: domain.StepRunStarted, Payload: mustJSON(domain.RunStartedPayload{Simulated: true})},
		domain.Step{Kind: domain.StepBudgetReserved, Payload: mustJSON(domain.BudgetReservedPayload{Micros: 30_000})},
		domain.Step{Kind: domain.StepToolCalled, Payload: mustJSON(domain.ToolCalledPayload{
			Tool: "crm.lookup", Effect: domain.EffectRead, ArgsDigest: digest(lookup("a").Args)})},
	)
	advanceToEnd(t, h, start)
	got := returned(t, h, start)
	if len(got) != 1 || got[0].ResultRef != world.Entries[0].AnswerRef || got[0].ErrorCode != "" {
		t.Errorf("results %+v, want the recorded answer and no unknown outcome", got)
	}
	steps, _ := h.ledger.Read(context.Background(), start.RunID, domain.FirstSeq)
	if reserved := mustFold(t, steps).Reserved; reserved.Micros != 0 {
		t.Errorf("still reserved %+v, want the call's reservation released", reserved)
	}
}
