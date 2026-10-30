package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// A run that fell back plans with the fallback pair for the rest of its life.
// Sticky because it is folded from the ledger: whichever worker picks the run
// up next reads the switch, not anybody's memory of the outage.

type inputPlanner struct {
	calls  int
	inputs []PlanInput
	answer Proposal
}

func (p *inputPlanner) Plan(_ context.Context, in PlanInput) (Proposal, error) {
	p.calls++
	p.inputs = append(p.inputs, in)
	return p.answer, nil
}

var (
	primaryPair  = domain.ModelPair{Provider: "anthropic", Model: "claude-opus-5"}
	fallbackPair = domain.ModelPair{Provider: "openai", Model: "gpt-test"}
)

func fellBack(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(domain.ModelFallbackPayload{
		From: primaryPair, To: fallbackPair, Attempts: 3,
		Failure: domain.FailureSummary{Code: "model_provider_overloaded", Provider: "anthropic", Retryable: true},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// seedFallenBack records a run that already switched, the way a worker would
// have left it for whichever worker comes next.
func seedFallenBack(t *testing.T, h *harness, payload []byte) {
	t.Helper()
	at := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	base := domain.Step{
		RunID: "run-1", Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: "v3", OnBehalfOf: "ana", At: at,
	}
	started := base
	started.Kind = domain.StepRunStarted
	switched := base
	switched.Kind = domain.StepModelFallback
	switched.Payload = payload
	for _, s := range []domain.Step{started, switched} {
		if _, err := h.ledger.Append(context.Background(), s); err != nil {
			t.Fatalf("seed %s: %v", s.Kind, err)
		}
	}
}

func withPlanners(h *harness) (primary, fallback *inputPlanner) {
	done := Proposal{Done: true, Outcome: "done"}
	primary, fallback = &inputPlanner{answer: done}, &inputPlanner{answer: done}
	h.runner.deps.Planner = primary
	h.runner.deps.Fallback = fallback
	return primary, fallback
}

func TestFold_modelFallback_recordsTheSwitch(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	seedFallenBack(t, h, fellBack(t))
	steps, err := h.ledger.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	state, err := Fold(steps)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	if state.Fallback == nil || state.Fallback.To != fallbackPair || state.Fallback.From != primaryPair {
		t.Fatalf("Fallback = %+v, want the recorded switch", state.Fallback)
	}
	if state.Phase != PhaseRunning {
		t.Errorf("Phase = %v, want running: a switch is not a pause", state.Phase)
	}
}

func TestFold_modelFallbackNamingNoPair_isRefused(t *testing.T) {
	t.Parallel()

	// A switch to nowhere cannot be planned with, and folding it as if it
	// were would send the next turn to whatever planner happened to be wired.
	h := newHarness(t)
	seedFallenBack(t, h, []byte(`{"from":{"provider":"anthropic","model":"claude-opus-5"},"failure":{"code":"x"}}`))
	steps, err := h.ledger.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, err := Fold(steps); err == nil {
		t.Fatal("folded a fallback that names no pair")
	}
}

func TestAdvance_afterAFallback_plansWithTheFallbackPlanner(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	primary, fallback := withPlanners(h)
	seedFallenBack(t, h, fellBack(t))

	if _, err := h.runner.Advance(context.Background(), h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if primary.calls != 0 {
		t.Errorf("primary planned %d times after the run fell back", primary.calls)
	}
	if fallback.calls != 1 {
		t.Errorf("fallback planned %d times, want 1", fallback.calls)
	}
}

func TestAdvance_afterAFallback_aStepsOwnModelIsNotSentToTheFallback(t *testing.T) {
	t.Parallel()

	// A step's model names a model of the declared provider. Sent to the
	// fallback it is a model that vendor does not have — or worse, one it
	// has under the same name and bills differently.
	h := newHarness(t)
	_, fallback := withPlanners(h)
	seedFallenBack(t, h, fellBack(t))
	start := h.start(t, generousBudget())
	start.Steps = []Envelope{{Name: "read", Reaches: []domain.ToolID{"crm.lookup"}, Model: "claude-haiku-5"}}

	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if fallback.calls != 1 || fallback.inputs[0].Model != "" {
		t.Fatalf("fallback inputs = %+v, want the fallback's own model", fallback.inputs)
	}
}

func TestAdvance_noFallbackRecorded_plansWithTheDeclaredPlanner(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	primary, fallback := withPlanners(h)
	start := h.start(t, generousBudget())
	start.Steps = []Envelope{{Name: "read", Reaches: []domain.ToolID{"crm.lookup"}, Model: "claude-haiku-5"}}

	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if fallback.calls != 0 || primary.calls != 1 {
		t.Fatalf("calls = primary %d / fallback %d, want the declared planner alone", primary.calls, fallback.calls)
	}
	if primary.inputs[0].Model != "claude-haiku-5" {
		t.Errorf("Model = %q, want the step's own", primary.inputs[0].Model)
	}
}

func TestAdvance_fellBackWithNoFallbackPlanner_failsRatherThanAskingThePrimary(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	primary, _ := withPlanners(h)
	h.runner.deps.Fallback = nil
	seedFallenBack(t, h, fellBack(t))

	_, err := h.runner.Advance(context.Background(), h.start(t, generousBudget()))
	if !errors.Is(err, ErrNoFallbackPlanner) {
		t.Fatalf("Advance = %v, want ErrNoFallbackPlanner", err)
	}
	if primary.calls != 0 {
		t.Error("asked the declared provider after the run had switched away from it")
	}
}

func TestAdvance_fellBackAndNowAutonomous_refusesToPlanWithTheFallback(t *testing.T) {
	t.Parallel()

	// Checked where the planner is chosen, on every turn: the stage can change
	// between the switch and any later claim.
	h := newHarness(t)
	primary, fallback := withPlanners(h)
	seedFallenBack(t, h, fellBack(t))
	start := h.start(t, generousBudget())
	start.Stage = domain.StageAutonomous

	_, err := h.runner.Advance(context.Background(), start)
	var unrehearsed FallbackUnrehearsedError
	if !errors.As(err, &unrehearsed) {
		t.Fatalf("Advance = %v, want FallbackUnrehearsedError", err)
	}
	if got := unrehearsed.Summary(); got.Code != domain.ParkFallbackUnrehearsed || got.Retryable {
		t.Errorf("summary = %+v, want a non-retryable %s", got, domain.ParkFallbackUnrehearsed)
	}
	if fallback.calls != 0 || primary.calls != 0 {
		t.Errorf("calls = primary %d / fallback %d, want neither asked", primary.calls, fallback.calls)
	}
}
