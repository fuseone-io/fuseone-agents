package compensate_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/compensate"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
)

// Performing calls the real tools. These tests are about it crossing the Gate,
// recording what it did, and telling the truth when an undo did not happen.

type effects map[domain.ToolID]domain.Effect

func (e effects) Effect(t domain.ToolID) (domain.Effect, bool) {
	effect, ok := e[t]
	return effect, ok
}

func (e effects) Dedupe(domain.ToolID) (domain.ToolDedupe, bool) {
	return domain.ToolDedupe{}, false
}

type recorder struct {
	calls  []engine.Call
	fail   map[domain.ToolID]bool
	broken map[domain.ToolID]bool
}

func (r *recorder) Reserve(context.Context, engine.Call) error { return nil }

func (r *recorder) Invoke(_ context.Context, c engine.Call) (engine.ToolResult, error) {
	r.calls = append(r.calls, c)
	if r.broken[c.Tool] {
		return engine.ToolResult{}, errors.New("connection refused")
	}
	return engine.ToolResult{Failed: r.fail[c.Tool], ErrorCode: "refused_by_tool"}, nil
}

type frozen struct{}

func (frozen) Now() time.Time { return time.Unix(1750000000, 0).UTC() }

// harness opens a run that charged and ordered, and returns what undoes it.
func harness(t *testing.T, tools *recorder) (compensate.Deps, engine.Start, []compensate.Act) {
	t.Helper()
	store := ledger.NewMemory()
	ctx := context.Background()

	start := engine.Start{
		RunID: "run-1", AgentID: "billing", VersionID: "v1",
		Scope: domain.Scope{Company: "acme", Area: "billing"},
		Pack:  gate.NewPack("crm.order", "crm.charge"),
		Stage: domain.StageAutonomous,
	}
	if _, err := store.Append(ctx, domain.Step{
		RunID: start.RunID, Kind: domain.StepRunStarted, Scope: start.Scope,
		AgentID: start.AgentID, VersionID: start.VersionID, At: frozen{}.Now(),
		Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("open run: %v", err)
	}

	return compensate.Deps{
			Ledger: store, Gate: gate.New(), Tools: tools, Clock: frozen{},
			Catalog: effects{
				"crm.charge.refund": domain.EffectFinancial,
				"crm.order.cancel":  domain.EffectWrite,
			},
		}, start, []compensate.Act{
			{Tool: "crm.charge", Seq: 4, Undo: "crm.charge.refund"},
			{Tool: "crm.order", Seq: 2, Undo: "crm.order.cancel"},
		}
}

func TestPerform_undoesEachActAndRecordsIt(t *testing.T) {
	t.Parallel()

	tools := &recorder{}
	deps, start, plan := harness(t, tools)

	out, err := compensate.Perform(context.Background(), deps, start, plan)
	if err != nil {
		t.Fatalf("perform: %v", err)
	}

	if len(tools.calls) != 2 ||
		tools.calls[0].Tool != "crm.charge.refund" || tools.calls[1].Tool != "crm.order.cancel" {
		t.Fatalf("called %+v, want the refund then the cancel", tools.calls)
	}
	for _, o := range out {
		if !o.Done {
			t.Errorf("%s not done: %s", o.Act.Undo, o.Why)
		}
	}

	steps, err := deps.Ledger.Read(context.Background(), start.RunID, 0)
	if err != nil {
		t.Fatalf("steps: %v", err)
	}
	compensated := 0
	for _, s := range steps {
		if s.Kind == domain.StepCompensated {
			compensated++
		}
	}
	if compensated != 2 {
		t.Errorf("recorded %d compensations, want 2", compensated)
	}
}

func TestPerform_oneUndoFails_theRestStillRun(t *testing.T) {
	t.Parallel()

	// Stopping at the first failure would leave more standing than carrying
	// on does, which is the opposite of the point. The acts are independent.
	tools := &recorder{fail: map[domain.ToolID]bool{"crm.charge.refund": true}}
	deps, start, plan := harness(t, tools)

	out, err := compensate.Perform(context.Background(), deps, start, plan)
	if err != nil {
		t.Fatalf("perform: %v", err)
	}

	if len(out) != 2 || out[0].Done || !out[1].Done {
		t.Fatalf("outcomes = %+v, want the refund failed and the cancel done", out)
	}
	if out[0].Why == "" {
		t.Error("a failed undo said nothing about why")
	}
}

func TestPerform_aBrokenToolLayer_isRecordedNotReturned(t *testing.T) {
	t.Parallel()

	// A connection that dropped mid-undo is the same fact as a refusal: the
	// thing is still standing. What must not happen is it going unrecorded.
	tools := &recorder{broken: map[domain.ToolID]bool{"crm.charge.refund": true}}
	deps, start, plan := harness(t, tools)

	out, err := compensate.Perform(context.Background(), deps, start, plan)
	if err != nil {
		t.Fatalf("perform: %v", err)
	}
	if out[0].Done {
		t.Errorf("outcome = %+v, want it not done", out[0])
	}

	steps, _ := deps.Ledger.Read(context.Background(), start.RunID, 0)
	found := false
	for _, s := range steps {
		if s.Kind == domain.StepCompensated {
			found = true
		}
	}
	if !found {
		t.Error("the failed undo left no step; nobody can see it is still standing")
	}
}

func TestPerform_anActWithNothingToUndoIt_isReportedNotSkipped(t *testing.T) {
	t.Parallel()

	tools := &recorder{}
	deps, start, _ := harness(t, tools)

	out, err := compensate.Perform(context.Background(), deps, start,
		[]compensate.Act{{Tool: "crm.email", Seq: 6}})
	if err != nil {
		t.Fatalf("perform: %v", err)
	}

	if len(out) != 1 || out[0].Done || out[0].Why == "" {
		t.Errorf("outcomes = %+v, want it reported as standing", out)
	}
	if len(tools.calls) != 0 {
		t.Errorf("called %+v with nothing to call", tools.calls)
	}
}

func TestPerform_leaseLost_noUndoLeavesTheProcess(t *testing.T) {
	t.Parallel()

	tools := &recorder{}
	deps, start, plan := harness(t, tools)
	// The worker that was asked to compensate lost its lease while suspended:
	// the store no longer recognises its epoch. Not one undo may fire — the
	// successor will run the same plan, and these calls act on the world.
	start.LeaseEpoch = 7

	_, err := compensate.Perform(context.Background(), deps, start, plan)
	if !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("Perform = %v, want %v", err, domain.ErrLeaseLost)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("undos fired = %d, want 0: the run belongs to the successor", len(tools.calls))
	}
	steps, err := deps.Ledger.Read(context.Background(), start.RunID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("ledger has %d steps, want the opening alone: a fenced worker records nothing", len(steps))
	}
}

func TestPerform_underALiveLease_undoesAndRecordsAsBefore(t *testing.T) {
	t.Parallel()

	tools := &recorder{}
	deps, start, plan := harness(t, tools)
	queue, ok := deps.Ledger.(interface {
		Claim(ctx context.Context, owner string, lease time.Duration) (domain.Claim, error)
	})
	if !ok {
		t.Fatal("the harness store lost its queue half")
	}
	claimed, err := queue.Claim(context.Background(), "w1", time.Minute)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	start.LeaseEpoch = claimed.Epoch

	out, err := compensate.Perform(context.Background(), deps, start, plan)
	if err != nil {
		t.Fatalf("Perform: %v", err)
	}
	if len(tools.calls) != len(plan) || len(out) != len(plan) {
		t.Fatalf("undos fired = %d, outcomes = %d, want %d of each", len(tools.calls), len(out), len(plan))
	}
}

func TestPerform_gateRefusalUnderALostLease_recordsNothing(t *testing.T) {
	t.Parallel()

	tools := &recorder{}
	deps, start, _ := harness(t, tools)
	start.LeaseEpoch = 7
	// An undo the catalogue does not know: the Gate refuses it, and the
	// refusal would be recorded — but the record belongs to the lease holder.
	plan := []compensate.Act{{Tool: "crm.charge", Seq: 4, Undo: "crm.mystery"}}

	_, err := compensate.Perform(context.Background(), deps, start, plan)
	if !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("Perform = %v, want %v", err, domain.ErrLeaseLost)
	}
	steps, readErr := deps.Ledger.Read(context.Background(), start.RunID, domain.FirstSeq)
	if readErr != nil {
		t.Fatalf("Read: %v", readErr)
	}
	if len(steps) != 1 {
		t.Fatalf("ledger has %d steps, want the opening alone", len(steps))
	}
}
