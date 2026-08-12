package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
)

// --- collaborators ----------------------------------------------------------

type flakyPlanner struct {
	failures int
	calls    int
	// proposal, when set, is what the planner asks for instead of finishing.
	// A run that finishes on its first turn never reaches the Gate, which is
	// where a ceiling is enforced.
	proposal *engine.Proposal
}

func (p *flakyPlanner) Plan(context.Context, engine.PlanInput) (engine.Proposal, error) {
	p.calls++
	if p.calls <= p.failures {
		return engine.Proposal{}, errors.New("model unreachable")
	}
	if p.proposal != nil {
		return *p.proposal, nil
	}
	return engine.Proposal{Done: true, Outcome: "completed"}, nil
}

// plannerThatFails never succeeds, so the supervision policy is what ends the
// run rather than the run ending itself.
func plannerThatFails() *flakyPlanner {
	return &flakyPlanner{failures: 99}
}

// wantsATool is a planner that proposes a call, so the Gate has something to
// rule on.
func wantsATool() *flakyPlanner {
	return &flakyPlanner{proposal: &engine.Proposal{
		Tool:     "crm.lookup",
		Args:     []byte(`{}`),
		Estimate: domain.Consumption{Micros: 5_000},
	}}
}

type noTools struct{}

func (noTools) Invoke(context.Context, Call) (engine.ToolResult, error) {
	return engine.ToolResult{}, nil
}

// Call is an alias so the stub reads naturally next to engine's port.
type Call = engine.Call

type emptyCatalog struct{}

func (emptyCatalog) Effect(domain.ToolID) (domain.Effect, bool) { return domain.EffectRead, true }

type staticSpecs struct {
	err     error
	planner engine.Planner
}

func (s staticSpecs) Resolve(context.Context, domain.AgentID, domain.VersionID) (Resolution, error) {
	if s.err != nil {
		return Resolution{}, s.err
	}
	return Resolution{
		Start: engine.Start{
			Pack:    gate.NewPack("crm.lookup"),
			Budget:  domain.Budget{Micros: 1_000_000, Steps: 50},
			Trigger: "worker",
		},
		Planner: s.planner,
	}, nil
}

type frozenClock struct{ t time.Time }

func (c frozenClock) Now() time.Time { return c.t }

// --- harness ----------------------------------------------------------------

type setup struct {
	worker  *Worker
	store   *ledger.Memory
	planner *flakyPlanner
}

// newSetup wires a worker whose spec resolution hands back the harness's own
// planner, which is what the worker substitutes per run.
func newSetup(t *testing.T, cfg Config, planner *flakyPlanner, specErr error) *setup {
	t.Helper()

	store := ledger.NewMemory()
	clock := frozenClock{t: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)}

	// The worker builds a runner per claim so the spec's own planner is used;
	// what it needs from us is the collaborators that do not vary by run.
	deps := engine.Deps{
		Ledger:  store,
		Gate:    gate.New(),
		Planner: planner,
		Tools:   noTools{},
		Catalog: emptyCatalog{},
		Clock:   clock,
	}

	specs := staticSpecs{err: specErr, planner: planner}

	cfg.Owner = "test-worker"
	return &setup{
		worker:  New(cfg, store, deps, specs, clock, slog.New(slog.DiscardHandler)),
		store:   store,
		planner: planner,
	}
}

// openRun puts a claimable run in the ledger.
func openRun(t *testing.T, store *ledger.Memory) {
	t.Helper()
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: "run-1", Kind: domain.StepRunStarted,
		Scope:   domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: "v3", At: time.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func phaseOf(t *testing.T, store *ledger.Memory) engine.Phase {
	t.Helper()
	steps, err := store.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	state, err := engine.Fold(steps)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	return state.Phase
}

// --- tests ------------------------------------------------------------------

func TestTurn_claimsAndAdvancesARun(t *testing.T) {
	t.Parallel()

	s := newSetup(t, Config{}, &flakyPlanner{}, nil)
	openRun(t, s.store)

	worked, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if !worked {
		t.Fatal("turn found no work with a claimable run in the ledger")
	}
	if got := phaseOf(t, s.store); got != engine.PhaseFinished {
		t.Errorf("Phase = %v, want finished", got)
	}
}

func TestTurn_emptyQueue_reportsNoWorkWithoutError(t *testing.T) {
	t.Parallel()

	s := newSetup(t, Config{}, &flakyPlanner{}, nil)

	worked, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Errorf("turn on an empty queue returned %v, want nil", err)
	}
	if worked {
		t.Error("turn reported work on an empty queue")
	}
}

func TestTurn_transientFailure_backsOffInsteadOfParking(t *testing.T) {
	t.Parallel()

	s := newSetup(t, Config{MaxAttempts: 3, BaseBackoff: time.Second}, &flakyPlanner{failures: 1}, nil)
	openRun(t, s.store)

	if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}

	// One failure is not a verdict. Parking here would take a human's
	// attention for something that resolves itself on the next attempt.
	if got := phaseOf(t, s.store); got == engine.PhaseParked {
		t.Error("a single transient failure parked the run")
	}
}

func TestTurn_repeatedFailures_parkForAHuman(t *testing.T) {
	t.Parallel()

	// Backoff in the past so every turn is immediately claimable again.
	s := newSetup(t, Config{MaxAttempts: 3, BaseBackoff: -time.Hour}, &flakyPlanner{failures: 99}, nil)
	openRun(t, s.store)

	ctx := context.Background()
	log := slog.New(slog.DiscardHandler)
	for range 3 {
		if _, err := s.worker.turn(ctx, log); err != nil {
			t.Fatalf("turn: %v", err)
		}
	}

	// A run that keeps failing must stop consuming the queue: retrying for
	// ever burns budget and hides the fault (PRD NF-14).
	if _, err := s.store.Claim(ctx, "other", time.Minute); !errors.Is(err, domain.ErrNoClaimableRun) {
		t.Errorf("run still claimable after %d failures, want parked", 3)
	}
}

func TestTurn_unresolvableSpec_parksImmediately(t *testing.T) {
	t.Parallel()

	s := newSetup(t, Config{MaxAttempts: 5}, &flakyPlanner{}, errors.New("no such version"))
	openRun(t, s.store)

	ctx := context.Background()
	if _, err := s.worker.turn(ctx, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}

	// Retrying cannot conjure a missing specification, so the backoff ladder
	// would only delay the moment someone notices.
	if _, err := s.store.Claim(ctx, "other", time.Minute); !errors.Is(err, domain.ErrNoClaimableRun) {
		t.Error("a run with an unresolvable spec is still being retried")
	}
}

func TestBackoff_doublesPerAttemptAndCaps(t *testing.T) {
	t.Parallel()

	cfg := Config{BaseBackoff: time.Second, MaxBackoff: 4 * time.Second}
	for attempt, want := range map[int]time.Duration{
		1: time.Second,
		2: 2 * time.Second,
		3: 4 * time.Second,
		4: 4 * time.Second, // capped
		9: 4 * time.Second,
	} {
		if got := backoff(cfg, attempt); got != want {
			t.Errorf("backoff(attempt=%d) = %v, want %v", attempt, got, want)
		}
	}
}

func TestRun_cancelled_stopsEveryWorkerInThePool(t *testing.T) {
	t.Parallel()

	s := newSetup(t, Config{Concurrency: 3, IdleWait: time.Millisecond}, &flakyPlanner{}, nil)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- s.worker.Run(ctx) }()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		// A pool that outlives its context blocks shutdown and gets SIGKILLed,
		// which is how a run dies mid-turn instead of finishing it.
		t.Fatal("Run did not return after the context was cancelled")
	}
}

func TestTurn_resolutionCarriesNoPlanner_runIsParkedRatherThanCrashed(t *testing.T) {
	t.Parallel()

	// A Specs implementation that resolves a version but forgets the planner
	// is a misconfiguration, not a transient fault. Handing the nil to the
	// runner would panic in the middle of a claimed run, leaving the lease to
	// expire and the next worker to do the same.
	store := ledger.NewMemory()
	clock := frozenClock{t: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)}
	w := New(Config{Owner: "test-worker", MaxAttempts: 5}, store,
		engine.Deps{Ledger: store, Gate: gate.New(), Tools: noTools{}, Catalog: emptyCatalog{}, Clock: clock},
		staticSpecs{}, clock, slog.New(slog.DiscardHandler))
	openRun(t, store)

	worked, err := w.turn(context.Background(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if !worked {
		t.Fatal("turn found no work with a claimable run in the ledger")
	}
	// Parked the same way an unresolvable spec is: retrying cannot conjure a
	// planner, so the backoff ladder would only delay someone noticing.
	if _, err := store.Claim(context.Background(), "other", time.Minute); !errors.Is(err, domain.ErrNoClaimableRun) {
		t.Error("a run whose spec has no planner is still being retried")
	}

	// And in the ledger, not only in the projection. The projection is not the
	// record: a trail that ends without saying the run was parked reads as a
	// run that stopped mid-turn, and every projection folded from it — the
	// diagram, the simulation report, replay — reports it as still going.
	if got := phaseOf(t, store); got != engine.PhaseParked {
		t.Errorf("ledger phase = %v, want the run parked", got)
	}
	if got := parkReason(t, store); got != "spec_unresolved" {
		t.Errorf("reason = %q, want what whoever unparks it needs", got)
	}
}

func parkReason(t *testing.T, store *ledger.Memory) string {
	t.Helper()
	steps, err := store.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	last := steps[len(steps)-1]
	if last.Kind != domain.StepParked {
		t.Fatalf("last step is %s, want the parking", last.Kind)
	}
	var p domain.ParkedPayload
	if err := json.Unmarshal(last.Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	return p.Reason
}

func TestTurn_attemptsExhausted_parksTheRunInTheLedgerToo(t *testing.T) {
	t.Parallel()

	// A run that fails for ever burns budget and hides the fault (NF-14). The
	// supervisor stops it, and the trail has to say that is what happened
	// rather than ending on the last failure.
	s := newSetup(t, Config{MaxAttempts: 1}, plannerThatFails(), nil)
	openRun(t, s.store)

	if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}
	if got := parkReason(t, s.store); got != "attempts_exhausted" {
		t.Errorf("reason = %q", got)
	}
}

// staticCeilings stands in for the configured scope budgets.
type staticCeilings struct {
	ceiling domain.Budget
	period  domain.Period
	spent   domain.Consumption
}

func (c staticCeilings) Resolve(context.Context, domain.Scope) (domain.Budget, domain.Period, error) {
	return c.ceiling, c.period, nil
}

func (c staticCeilings) SpentSince(context.Context, domain.Scope, time.Time) (domain.Consumption, error) {
	return c.spent, nil
}

func TestTurn_scopeCeilingAlreadySpent_parksTheRunResumably(t *testing.T) {
	t.Parallel()

	// The agent's own ceiling is generous; its area has spent its month. The
	// run must stop, and stop resumably — raising the ceiling resumes from the
	// exact step rather than repeating an effect (PRD FO-04).
	s := newSetup(t, Config{MaxAttempts: 5}, wantsATool(), nil)
	s.worker.WithCeilings(staticCeilings{
		ceiling: domain.Budget{Micros: 100_000},
		period:  domain.PeriodMonthly,
		spent:   domain.Consumption{Micros: 100_000},
	})
	openRun(t, s.store)

	ctx := context.Background()
	if _, err := s.worker.turn(ctx, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}

	if got := phaseOf(t, s.store); got != engine.PhaseParked {
		t.Errorf("phase = %v, want the run parked at its area's ceiling", got)
	}
}

func TestTurn_scopeCeilingWithHeadroom_letsTheRunProceed(t *testing.T) {
	t.Parallel()

	s := newSetup(t, Config{MaxAttempts: 5}, wantsATool(), nil)
	s.worker.WithCeilings(staticCeilings{
		ceiling: domain.Budget{Micros: 1_000_000},
		period:  domain.PeriodMonthly,
		spent:   domain.Consumption{Micros: 10_000},
	})
	openRun(t, s.store)

	ctx := context.Background()
	if _, err := s.worker.turn(ctx, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}

	if got := phaseOf(t, s.store); got == engine.PhaseParked {
		t.Error("a run parked while its area still had budget")
	}
}

func TestTurn_noCeilingConfigured_usesTheAgentsOwn(t *testing.T) {
	t.Parallel()

	// An installation that configured nothing runs on the ceilings in each
	// agent's specification, exactly as before scope budgets existed.
	s := newSetup(t, Config{MaxAttempts: 5}, wantsATool(), nil)
	s.worker.WithCeilings(staticCeilings{})
	openRun(t, s.store)

	ctx := context.Background()
	if _, err := s.worker.turn(ctx, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}
	if got := phaseOf(t, s.store); got == engine.PhaseParked {
		t.Error("a run parked with no ceiling configured anywhere")
	}
}
