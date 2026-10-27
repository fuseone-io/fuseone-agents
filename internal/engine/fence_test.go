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
	// readCap, when non-zero, truncates reads to that many steps: the stale
	// fold of a worker that read the run before a later write landed.
	readCap atomic.Int64
	// ambiguous makes fenced appends commit and then answer context.Canceled:
	// the write landed, the caller cannot know it did.
	ambiguous atomic.Bool
}

func (l *epochLedger) Read(ctx context.Context, runID domain.RunID, fromSeq int64) ([]domain.Step, error) {
	steps, err := l.Memory.Read(ctx, runID, fromSeq)
	if cap := l.readCap.Load(); cap > 0 && int64(len(steps)) > cap {
		steps = steps[:cap]
	}
	return steps, err
}

func (l *epochLedger) AppendIfLeaseAndHead(
	ctx context.Context, epoch int64, head domain.StepRef, s domain.Step,
) (domain.Step, error) {
	if epoch != l.current.Load() {
		return domain.Step{}, domain.ErrLeaseLost
	}
	if l.ambiguous.Load() {
		// The ambiguous commit: the write reaches the store — a commit does
		// not care that its caller was cancelled — but the answer is lost.
		if _, err := l.Memory.AppendIfHead(context.Background(), head, s); err != nil {
			return domain.Step{}, err
		}
		return domain.Step{}, context.Canceled
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
	return newFenceHarnessWith(t, &scriptedPlanner{proposals: proposals})
}

func newFenceHarnessWith(t *testing.T, planner Planner) (*harness, *epochLedger) {
	t.Helper()
	l := &epochLedger{Memory: ledger.NewMemory()}
	l.current.Store(1)
	content := NewMemoryContent()
	tools := &countingTools{content: content, body: []byte(`{"ok":true}`)}

	h := &harness{ledger: l.Memory, tools: tools, content: content}
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

func TestAdvance_plannedStepFencedByTheLease_theBilledSpendStillLands(t *testing.T) {
	t.Parallel()

	h, l := newFenceHarness(t, Proposal{
		Done: true, Outcome: "completed",
		Cost: domain.Cost{Micros: 12_345, InputTokens: 100, OutputTokens: 20},
	})
	// The successor claims while the model call is in flight: the opening step
	// landed under this worker's lease, the planning record no longer can.
	l.afterAppend = func(kind domain.StepKind) {
		if kind == domain.StepRunStarted {
			l.current.Store(2)
		}
	}

	_, err := h.runner.Advance(context.Background(), h.fencedStart(t))
	if !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("Advance = %v, want %v", err, domain.ErrLeaseLost)
	}

	// The call was billed by the provider whatever the fence says. Prometheus
	// saw it; the ledger — budget, FinOps, the trail — must see it too, and
	// without the stale proposal riding along.
	steps, err := l.Memory.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var micros int64
	for _, s := range steps {
		micros += s.Cost.Micros
		if s.Kind == domain.StepRunFinished {
			t.Fatalf("the fenced turn still finished the run: %v", s)
		}
	}
	if micros != 12_345 {
		t.Fatalf("ledger micros = %d, want 12345: the billed call vanished from the record", micros)
	}
}

func TestAdvance_plannedStepRefusedByAMovedHead_theBilledSpendStillLands(t *testing.T) {
	t.Parallel()

	h, l := newFenceHarness(t, Proposal{
		Done: true, Outcome: "completed", Cost: domain.Cost{Micros: 12_345},
	})
	// An approval or an abandonment lands while the model call is in flight:
	// same lease, different head.
	l.afterAppend = func(kind domain.StepKind) {
		if kind != domain.StepRunStarted {
			return
		}
		if _, err := l.Memory.Append(context.Background(), domain.Step{
			RunID: "run-1", Kind: domain.StepResumed,
			Scope:   domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", At: time.Now(), Payload: []byte(`{"by":"ana"}`),
		}); err != nil {
			t.Errorf("interloper append: %v", err)
		}
	}

	_, err := h.runner.Advance(context.Background(), h.fencedStart(t))
	if !errors.Is(err, domain.ErrHeadMoved) {
		t.Fatalf("Advance = %v, want %v", err, domain.ErrHeadMoved)
	}
	steps, err := l.Memory.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var micros int64
	for _, s := range steps {
		micros += s.Cost.Micros
	}
	if micros != 12_345 {
		t.Fatalf("ledger micros = %d, want 12345: the billed call vanished from the record", micros)
	}
}

// cancellingPlanner is the model call that completes — and is billed — at the
// exact moment the lease is lost: the cancel lands first, the answer follows.
type cancellingPlanner struct {
	cancel context.CancelCauseFunc
	cost   domain.Cost
}

func (p *cancellingPlanner) Plan(context.Context, PlanInput) (Proposal, error) {
	p.cancel(domain.ErrLeaseLost)
	return Proposal{Done: true, Outcome: "completed", Cost: p.cost}, nil
}

func TestAdvance_cancelCrossesABilledResponse_theSpendStillLands(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	planner := &cancellingPlanner{cancel: cancel, cost: domain.Cost{Micros: 12_345}}
	h, l := newFenceHarnessWith(t, planner)

	if _, err := h.runner.Advance(ctx, h.fencedStart(t)); err == nil {
		t.Fatal("Advance = nil, want the cancellation to end the turn")
	}

	// The provider billed the call whether or not the cancel won the race to
	// the ledger. A spend write riding the turn's own context dies with it.
	steps, err := l.Memory.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var micros int64
	for _, s := range steps {
		micros += s.Cost.Micros
	}
	if micros != 12_345 {
		t.Fatalf("ledger micros = %d, want 12345: the cancellation swallowed the invoice", micros)
	}
}

func TestAdvance_twoAttemptsBilledOverTheSameHead_bothSpendsLand(t *testing.T) {
	t.Parallel()

	h, l := newFenceHarness(t,
		Proposal{Done: true, Outcome: "completed", Cost: domain.Cost{Micros: 100}},
		Proposal{Done: true, Outcome: "completed", Cost: domain.Cost{Micros: 200}},
	)
	if _, err := l.Memory.Append(context.Background(), domain.Step{
		RunID: "run-1", Kind: domain.StepRunStarted,
		Scope:   domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", At: time.Now(), Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("open run: %v", err)
	}
	// Neither epoch is current any more: both attempts were fenced, and each
	// made its own billed call. The second worker folded the run before the
	// first one's spend record landed — the ordinary interleaving, since the
	// claim that fenced the first happens before its fallback write.
	l.current.Store(3)

	start := h.fencedStart(t) // epoch 1
	if _, err := h.runner.Advance(context.Background(), start); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("first Advance = %v, want %v", err, domain.ErrLeaseLost)
	}
	l.readCap.Store(1) // the second attempt's fold predates the first spend
	start.LeaseEpoch = 2
	if _, err := h.runner.Advance(context.Background(), start); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("second Advance = %v, want %v", err, domain.ErrLeaseLost)
	}

	steps, err := l.Memory.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var micros int64
	for _, s := range steps {
		micros += s.Cost.Micros
	}
	if micros != 300 {
		t.Fatalf("ledger micros = %d, want 300: two distinct charges, two records", micros)
	}
}

func TestAdvance_ambiguousCommitOnTheBilledRecord_theSpendLandsExactlyOnce(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	planner := &cancellingPlanner{cancel: cancel, cost: domain.Cost{Micros: 12_345}}
	h, l := newFenceHarnessWith(t, planner)
	if _, err := l.Memory.Append(context.Background(), domain.Step{
		RunID: "run-1", Kind: domain.StepRunStarted,
		Scope:   domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", At: time.Now(), Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("open run: %v", err)
	}
	// The primary write commits, but the cancellation swallows its answer:
	// the caller cannot tell a landed write from a lost one. Recording again
	// "to be safe" bills the same call twice.
	l.ambiguous.Store(true)

	if _, err := h.runner.Advance(ctx, h.fencedStart(t)); err == nil {
		t.Fatal("Advance = nil, want the cancellation to end the turn")
	}

	steps, err := l.Memory.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var micros int64
	for _, s := range steps {
		micros += s.Cost.Micros
	}
	if micros != 12_345 {
		t.Fatalf("ledger micros = %d, want 12345 exactly once: an ambiguous commit was double-recorded", micros)
	}
}
