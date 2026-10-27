package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
)

// epochLedger wraps the in-memory store with a lease epoch the test moves by
// hand, so the lease can be pulled out from under a turn at an exact moment —
// the thing a real successor does by claiming between two of this worker's
// instructions.
type epochLedger struct {
	*ledger.Memory
	current atomic.Int64
	// afterAppend runs after a step lands, before the call returns — the seam
	// where "the worker was suspended right here" is expressed.
	afterAppend func(kind domain.StepKind)
}

func (l *epochLedger) AppendIfLeaseAndHead(
	ctx context.Context, epoch int64, head domain.StepRef, s domain.Step,
) (domain.Step, error) {
	if epoch != l.current.Load() {
		return domain.Step{}, domain.ErrLeaseLost
	}
	var sealed domain.Step
	var err error
	if head == (domain.StepRef{}) {
		sealed, err = l.Memory.Append(ctx, s)
	} else {
		sealed, err = l.Memory.AppendIfHead(ctx, head, s)
	}
	if err == nil && l.afterAppend != nil {
		l.afterAppend(sealed.Kind)
	}
	return sealed, err
}

func (l *epochLedger) Fence(
	ctx context.Context, runID domain.RunID, epoch int64, head domain.StepRef,
) error {
	if epoch != l.current.Load() {
		return domain.ErrLeaseLost
	}
	return l.Memory.Fence(ctx, runID, 0, head)
}

func newFenceHarness(t *testing.T, proposals ...Proposal) (*harness, *epochLedger) {
	t.Helper()
	l := &epochLedger{Memory: ledger.NewMemory()}
	l.current.Store(1)
	planner := &scriptedPlanner{proposals: proposals}
	content := NewMemoryContent()
	tools := &countingTools{content: content, body: []byte(`{"ok":true}`)}

	h := &harness{ledger: l.Memory, planner: planner, tools: tools, content: content}
	h.runner = NewRunner(Deps{
		Ledger:  l,
		Content: content,
		Gate:    gate.New(),
		Planner: planner,
		Tools:   tools,
		Catalog: staticCatalog{"crm.lookup": domain.EffectRead},
		Clock:   fixedClock{t: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)},
	})
	return h, l
}

func (h *harness) fencedStart(t *testing.T) Start {
	t.Helper()
	start := h.start(t, domain.Budget{Micros: 1_000_000, Steps: 50})
	start.LeaseEpoch = 1
	return start
}

func TestAdvance_leaseLostBetweenTheCallRecordAndTheEffect_theEffectNeverLeaves(t *testing.T) {
	t.Parallel()

	h, l := newFenceHarness(t, Proposal{
		Tool: "crm.lookup", Args: []byte(`{}`),
		Estimate: domain.Consumption{Micros: 5_000},
	})
	// The suspension happens after the call is recorded and before the tool
	// fires: the successor claims exactly there. Renewal cannot see it — the
	// next tick has not come — so only a fence at the effect itself can.
	l.afterAppend = func(kind domain.StepKind) {
		if kind == domain.StepToolCalled {
			l.current.Store(2)
		}
	}

	_, err := h.runner.Advance(context.Background(), h.fencedStart(t))
	if !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("Advance = %v, want %v", err, domain.ErrLeaseLost)
	}
	if n := len(h.tools.invocations); n != 0 {
		t.Fatalf("external effects = %d, want 0: the run belongs to the successor", n)
	}
}

func TestAdvance_leaseAlreadyLost_noStepLandsAndNoEffectLeaves(t *testing.T) {
	t.Parallel()

	h, l := newFenceHarness(t, Proposal{Done: true, Outcome: "completed"})
	// The successor claimed while this worker was suspended, before it wrote
	// anything. The head is untouched — only the epoch says the run is gone.
	l.current.Store(2)

	_, err := h.runner.Advance(context.Background(), h.fencedStart(t))
	if !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("Advance = %v, want %v", err, domain.ErrLeaseLost)
	}
	steps, readErr := l.Memory.Read(context.Background(), "run-1", domain.FirstSeq)
	if readErr != nil && !errors.Is(readErr, ledger.ErrNotFound) {
		t.Fatalf("Read: %v", readErr)
	}
	if len(steps) != 0 {
		t.Fatalf("ledger has %d steps, want none from the fenced worker: %v", len(steps), steps)
	}
	if n := len(h.tools.invocations); n != 0 {
		t.Fatalf("external effects = %d, want 0", n)
	}
}
