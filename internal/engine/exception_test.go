package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
)

/*
The exception a step declares, and who decides it happened.

NT-003 left that open and this settles the conservative half of it: the model
is told what the author wrote, and may answer that it happened. Stopping takes
no effect, so a run that stops early cannot do anything a run that carried on
would not have — which is why this can be the model's call while every effect
stays the Gate's.

What is recorded is that the model said so, in the author's own words. Nobody
claims it was verified, and the trail should not read as though anybody did.
*/

func TestPlan_atAStepWithAnException_theModelIsToldWhatItSays(t *testing.T) {
	t.Parallel()

	seen, r := planning(t)
	start := Start{
		RunID: "run-1",
		Pack:  gate.NewPack("crm.lookup", "crm.reply"),
		Steps: []Envelope{
			{
				Name:      "Encontrar o cliente",
				Reaches:   []domain.ToolID{"crm.lookup"},
				StopsWhen: "não encontrar o cliente",
			},
			{Name: "Responder", Reaches: []domain.ToolID{"crm.reply"}},
		},
	}

	if _, err := r.plan(context.Background(), State{}, start); err != nil {
		t.Fatalf("plan: %v", err)
	}

	if seen.in.Step != "Encontrar o cliente" {
		t.Errorf("step = %q, want the one the run is at", seen.in.Step)
	}
	if seen.in.StopsWhen != "não encontrar o cliente" {
		t.Errorf("exception = %q, want the author's words", seen.in.StopsWhen)
	}
}

// The comment on PlanInput has always said the model is offered only what it
// may call, so that an unavailable tool cannot be proposed at all. With steps
// declared it was offered the whole pack, proposed what the step forbade, and
// spent a turn being refused.
func TestPlan_withStepsDeclared_offersOnlyWhatTheStepReaches(t *testing.T) {
	t.Parallel()

	seen, r := planning(t)
	start := Start{
		RunID: "run-1",
		Pack:  gate.NewPack("crm.lookup", "crm.reply"),
		Steps: []Envelope{
			{Name: "Encontrar", Reaches: []domain.ToolID{"crm.lookup"}},
			{Name: "Responder", Reaches: []domain.ToolID{"crm.reply"}},
		},
	}

	if _, err := r.plan(context.Background(), State{}, start); err != nil {
		t.Fatalf("plan: %v", err)
	}

	// Both, because forward stays open: the run may reach the step it is in
	// and every one after it, never the ones it has left behind.
	if len(seen.in.Tools) != 2 {
		t.Errorf("offered %v, want this step onwards", seen.in.Tools)
	}
}

// planning is a runner that does nothing but ask, and the planner that
// remembers what it was asked.
func planning(t *testing.T) (*recordingPlanner, *Runner) {
	t.Helper()

	// A run has to exist for its transcript to be read back: the model's view
	// is folded from the ledger every turn rather than held anywhere.
	l := ledger.NewMemory()
	if _, err := l.Append(context.Background(), domain.Step{
		RunID: "run-1", Kind: domain.StepRunStarted, AgentID: "triage",
		Scope:   domain.Scope{Company: "acme", Area: "cx"},
		Payload: []byte(`{"trigger":"test"}`),
	}); err != nil {
		t.Fatalf("open the run: %v", err)
	}

	seen := &recordingPlanner{}
	return seen, NewRunner(Deps{
		Ledger:  l,
		Content: NewMemoryContent(),
		Gate:    gate.New(),
		Planner: seen,
	})
}

type recordingPlanner struct{ in PlanInput }

func (p *recordingPlanner) Plan(_ context.Context, in PlanInput) (Proposal, error) {
	p.in = in
	return Proposal{Done: true}, nil
}

// A run that ends on its step's exception says which one, in the words the
// author wrote. Counted as finished rather than failed: giving up where the
// author said to is the agent doing as it was told.
func TestRun_stoppedByTheStepsException_recordsItVerbatim(t *testing.T) {
	t.Parallel()

	h := newHarness(t, Proposal{
		Done:      true,
		Outcome:   "no customer",
		StoppedBy: "não encontrar o cliente",
	})
	start := h.start(t, domain.Budget{Micros: 500_000, Steps: 10})

	if _, err := h.runner.Advance(context.Background(), start); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	steps, err := h.ledger.Read(context.Background(), start.RunID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	last := steps[len(steps)-1]
	if last.Kind != domain.StepRunFinished {
		t.Fatalf("last step = %s, want the run finished", last.Kind)
	}

	var finished domain.RunFinishedPayload
	if err := json.Unmarshal(last.Payload, &finished); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if finished.StoppedBy != "não encontrar o cliente" {
		t.Errorf("stopped_by = %q, want the author's words", finished.StoppedBy)
	}
}
