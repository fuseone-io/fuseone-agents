package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
)

// --- collaborators ----------------------------------------------------------

type flakyPlanner struct {
	failures int
	calls    int
	err      error
	// proposal, when set, is what the planner asks for instead of finishing.
	// A run that finishes on its first turn never reaches the Gate, which is
	// where a ceiling is enforced.
	proposal *engine.Proposal
}

func (p *flakyPlanner) Plan(context.Context, engine.PlanInput) (engine.Proposal, error) {
	p.calls++
	if p.calls <= p.failures {
		if p.err != nil {
			return engine.Proposal{}, p.err
		}
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

func (noTools) Reserve(context.Context, Call) error { return nil }

func (noTools) Invoke(context.Context, Call) (engine.ToolResult, error) {
	return engine.ToolResult{}, nil
}

// Call is an alias so the stub reads naturally next to engine's port.
type Call = engine.Call

type emptyCatalog struct{}

func (emptyCatalog) Effect(domain.ToolID) (domain.Effect, bool) { return domain.EffectRead, true }
func (emptyCatalog) Dedupe(domain.ToolID) (domain.ToolDedupe, bool) {
	return domain.ToolDedupe{}, false
}

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
		Ledger: store,
		// A run's closing answer goes to the content store, and the runner
		// refuses to finish without one rather than writing it into the chain.
		Content: engine.NewMemoryContent(),
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

func TestTurn_recordsMetricsForClaimsAndResultingPhase(t *testing.T) {
	t.Parallel()

	reg := NewMetricsRegistry()
	s := newSetup(t, Config{}, &flakyPlanner{}, nil)
	s.worker.WithMetrics(reg.Pool("runs", 4))
	openRun(t, s.store)

	if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}

	body := metricsBody(reg)
	for _, want := range []string{
		`fuseone_worker_claims_total{pool="runs",result="claimed"} 1`,
		`fuseone_worker_advances_total{pool="runs",phase="finished"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
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

func TestTurn_spentPlanFailure_putsTheSpendOnTheLedgerAndRetries(t *testing.T) {
	t.Parallel()

	spent := &engine.SpentPlanError{
		Cost: domain.Cost{InputTokens: 1, OutputTokens: 1,
			CacheReadTokens: 320, CacheWriteTokens: 640, Micros: 30},
		Prompt:   domain.PromptInputBreakdown{Unit: "bytes", Instructions: 12},
		Provider: "anthropic", Model: "claude-opus-5-5", ServedModel: "claude-opus-5",
		Err: &model.ProviderError{Provider: "anthropic", Code: model.CodeProviderUnavailable, Retryable: true},
	}
	reg := NewMetricsRegistry()
	s := newSetup(t, Config{MaxAttempts: 3, BaseBackoff: time.Second}, &flakyPlanner{failures: 1, err: spent}, nil)
	s.worker.WithMetrics(reg.Pool("runs", 4))
	openRun(t, s.store)

	if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}

	if got := phaseOf(t, s.store); got == engine.PhaseParked {
		t.Error("a retryable spent failure parked the run")
	}
	// The provider billed that call whether or not the turn survived it: the
	// ledger holds the spend exactly once, with the pair it belongs to.
	runs, err := s.store.Runs(context.Background())
	if err != nil || len(runs) != 1 {
		t.Fatalf("Runs = %v, %v", runs, err)
	}
	steps, err := s.store.Read(context.Background(), runs[0], domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	recorded := 0
	for _, step := range steps {
		if step.Kind == domain.StepPlanned && step.Cost.Micros == 30 {
			recorded++
			var payload domain.PlannedPayload
			if err := json.Unmarshal(step.Payload, &payload); err != nil ||
				payload.Provider != "anthropic" || payload.Model != "claude-opus-5-5" ||
				payload.ServedModel != "claude-opus-5" ||
				payload.Prompt == nil {
				t.Errorf("payload = %s, want both models and the prompt breakdown", step.Payload)
			}
		}
	}
	if recorded != 1 {
		t.Errorf("spend steps = %d, want exactly one", recorded)
	}
	// Prometheus is the other projection: the billed attempt's cache tokens
	// must land there too, or dashboards under-read exactly the runs that
	// are burning money on retries.
	body := metricsBody(reg)
	for _, want := range []string{
		`fuseone_worker_model_cache_tokens_total{pool="runs",operation="read"} 320`,
		`fuseone_worker_model_cache_tokens_total{pool="runs",operation="write"} 640`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q", want)
		}
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

func parkPayload(t *testing.T, store *ledger.Memory) domain.ParkedPayload {
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
	return p
}

func parkReason(t *testing.T, store *ledger.Memory) string {
	t.Helper()
	return parkPayload(t, store).Reason
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

func TestTurn_providerFailure_parksWithTheStableProviderSummary(t *testing.T) {
	t.Parallel()

	planner := &flakyPlanner{
		failures: 99,
		err: &model.ProviderError{
			Provider:  "anthropic",
			Code:      model.CodeProviderOverloaded,
			Status:    529,
			RequestID: "req_011CeAaYZkdUe63yaSu5CxCX",
			Retryable: true,
		},
	}
	s := newSetup(t, Config{MaxAttempts: 1}, planner, nil)
	openRun(t, s.store)

	if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}

	p := parkPayload(t, s.store)
	if p.Reason != model.CodeProviderOverloaded {
		t.Fatalf("reason = %q, want the typed provider code", p.Reason)
	}
	if p.Failure == nil {
		t.Fatal("parked payload has no provider failure summary")
	}
	if p.Failure.Provider != "anthropic" || p.Failure.Status != 529 ||
		p.Failure.RequestID != "req_011CeAaYZkdUe63yaSu5CxCX" || !p.Failure.Retryable {
		t.Errorf("failure = %+v, want the stable provider summary", *p.Failure)
	}
}

func TestTurn_dedupeInFlightParksWithTheStableEngineCode(t *testing.T) {
	t.Parallel()

	planner := &flakyPlanner{
		failures: 99,
		err:      engine.DedupeInFlightError{},
	}
	s := newSetup(t, Config{MaxAttempts: 1}, planner, nil)
	openRun(t, s.store)

	if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}

	p := parkPayload(t, s.store)
	if p.Reason != engine.CodeDedupeInFlight {
		t.Fatalf("reason = %q, want %s", p.Reason, engine.CodeDedupeInFlight)
	}
	if p.Failure == nil || p.Failure.Code != engine.CodeDedupeInFlight || !p.Failure.Retryable {
		t.Fatalf("failure = %+v, want retryable %s", p.Failure, engine.CodeDedupeInFlight)
	}
}

func TestTurn_nonRetryableProviderFailureParksWithoutBurningAttempts(t *testing.T) {
	t.Parallel()

	planner := &flakyPlanner{
		failures: 99,
		err: &model.ProviderError{
			Provider:  "anthropic",
			Code:      model.CodeAuthFailed,
			Status:    401,
			RequestID: "req_auth",
			Retryable: false,
		},
	}
	s := newSetup(t, Config{MaxAttempts: 5}, planner, nil)
	openRun(t, s.store)

	if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}

	if planner.calls != 1 {
		t.Fatalf("planner calls = %d, want one failed attempt", planner.calls)
	}
	if got := phaseOf(t, s.store); got != engine.PhaseParked {
		t.Fatalf("phase = %s, want parked on a definitive provider failure", got)
	}
	p := parkPayload(t, s.store)
	if p.Reason != model.CodeAuthFailed || p.Attempts != 1 {
		t.Fatalf("parked = %+v, want auth failure parked after one attempt", p)
	}
	if p.Failure == nil || p.Failure.Retryable ||
		p.Failure.Code != model.CodeAuthFailed || p.Failure.Provider != "anthropic" {
		t.Fatalf("failure = %+v, want the non-retryable provider summary", p.Failure)
	}
	if _, err := s.store.Claim(context.Background(), "other", time.Minute); !errors.Is(err, domain.ErrNoClaimableRun) {
		t.Fatalf("Claim after non-retryable provider failure = %v, want no retry queued", err)
	}
}

func TestTurn_failureMetricsUseStableCodesInsteadOfErrorText(t *testing.T) {
	t.Parallel()

	reg := NewMetricsRegistry()
	planner := &flakyPlanner{
		failures: 99,
		err: &model.ProviderError{
			Provider:  "anthropic",
			Code:      model.CodeAuthFailed,
			Status:    401,
			RequestID: "req_auth",
			Message:   "secret diagnostic text",
			Retryable: false,
		},
	}
	s := newSetup(t, Config{MaxAttempts: 5}, planner, nil)
	s.worker.WithMetrics(reg.Pool("runs", 4))
	openRun(t, s.store)

	if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}

	body := metricsBody(reg)
	if !strings.Contains(body,
		`fuseone_worker_advance_failures_total{pool="runs",reason="model_auth_failed",parked="true"} 1`) {
		t.Fatalf("metrics body did not record the stable failure code:\n%s", body)
	}
	if strings.Contains(body, "secret diagnostic text") || strings.Contains(body, "req_auth") {
		t.Fatalf("metrics body contains diagnostic text:\n%s", body)
	}
}

func metricsBody(reg *MetricsRegistry) string {
	rec := httptest.NewRecorder()
	reg.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rec.Body.String()
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

// --- lease fencing -----------------------------------------------------------

// fencedQueue wraps the in-memory store so a test can watch renewals, force a
// lost lease, and count releases, without reaching into the store's internals.
type fencedQueue struct {
	*ledger.Memory
	renewed    chan struct{}
	renewErr   error
	renewNoop  bool
	releaseErr error
	releases   atomic.Int32
	mu         sync.Mutex
	outcomes   []domain.ClaimOutcome
}

func (q *fencedQueue) Renew(ctx context.Context, runID domain.RunID, epoch int64, lease time.Duration) error {
	if q.renewErr != nil {
		return q.renewErr
	}
	if q.renewNoop {
		return nil
	}
	err := q.Memory.Renew(ctx, runID, epoch, lease)
	select {
	case q.renewed <- struct{}{}:
	default:
	}
	return err
}

func (q *fencedQueue) Release(ctx context.Context, runID domain.RunID, outcome domain.ClaimOutcome) error {
	q.releases.Add(1)
	q.mu.Lock()
	q.outcomes = append(q.outcomes, outcome)
	q.mu.Unlock()
	if q.releaseErr != nil {
		return q.releaseErr
	}
	return q.Memory.Release(ctx, runID, outcome)
}

// gatedPlanner holds the turn open until the test lets go — or the context is
// cancelled, which it reports on sawCancel. It announces on entered that the
// turn has folded the run and is now planning.
type gatedPlanner struct {
	entered   chan struct{}
	release   chan struct{}
	sawCancel chan struct{}
}

func (p *gatedPlanner) Plan(ctx context.Context, _ engine.PlanInput) (engine.Proposal, error) {
	select {
	case p.entered <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		close(p.sawCancel)
		return engine.Proposal{}, ctx.Err()
	case <-p.release:
		return engine.Proposal{Done: true, Outcome: "completed"}, nil
	}
}

func newGatedPlanner() *gatedPlanner {
	return &gatedPlanner{
		entered:   make(chan struct{}, 1),
		release:   make(chan struct{}),
		sawCancel: make(chan struct{}),
	}
}

func newFencedWorker(t *testing.T, cfg Config, planner engine.Planner, queue Queue, store *ledger.Memory) *Worker {
	t.Helper()
	clock := frozenClock{t: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)}
	deps := engine.Deps{
		Ledger: store, Content: engine.NewMemoryContent(), Gate: gate.New(),
		Planner: planner, Tools: noTools{}, Catalog: emptyCatalog{}, Clock: clock,
	}
	cfg.Owner = "test-worker"
	return New(cfg, queue, deps, staticSpecs{planner: planner}, clock, slog.New(slog.DiscardHandler))
}

func TestTurn_outlastingTheRenewInterval_renewsTheLease(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	q := &fencedQueue{Memory: store, renewed: make(chan struct{}, 1)}
	p := newGatedPlanner()
	w := newFencedWorker(t, Config{Lease: 90 * time.Millisecond}, p, q, store)
	openRun(t, store)

	done := make(chan error, 1)
	go func() {
		_, err := w.turn(context.Background(), slog.New(slog.DiscardHandler))
		done <- err
	}()

	select {
	case <-q.renewed:
		// The lease was extended while the turn was still planning: a slow
		// turn no longer silently outlives its claim.
	case <-time.After(5 * time.Second):
		t.Fatal("no renewal while the turn ran past the renew interval")
	}
	close(p.release)
	if err := <-done; err != nil {
		t.Fatalf("turn: %v", err)
	}
}

func TestTurn_leaseLost_cancelsTheTurnAndWalksAwayQuietly(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	q := &fencedQueue{Memory: store, renewed: make(chan struct{}, 1), renewErr: domain.ErrLeaseLost}
	p := newGatedPlanner() // nothing closes release: only a cancel can end the plan
	w := newFencedWorker(t, Config{Lease: 60 * time.Millisecond}, p, q, store)
	openRun(t, store)

	worked, err := w.turn(context.Background(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if !worked {
		t.Fatal("turn reported no work after claiming the run")
	}

	select {
	case <-p.sawCancel:
		// The in-flight model call was fenced: the worker that lost its lease
		// stopped acting on the run instead of racing its successor.
	default:
		t.Fatal("the planner kept running after the lease was lost")
	}
	if got := phaseOf(t, store); got == engine.PhaseParked {
		t.Error("a lost lease parked the run — that is the successor's story to tell")
	}
	if got := q.releases.Load(); got != 0 {
		t.Errorf("releases = %d, want none: the lease is no longer this worker's to hand back", got)
	}
}

func TestTurn_releaseFindsTheLeaseAlreadyLost_turnEndsWithoutError(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	q := &fencedQueue{Memory: store, renewed: make(chan struct{}, 1), releaseErr: domain.ErrLeaseLost}
	s := &flakyPlanner{}
	w := newFencedWorker(t, Config{}, s, q, store)
	openRun(t, store)

	// The turn finished, but a successor claimed the run in between: the stale
	// outcome has nowhere to go, and that is an ending, not a worker fault.
	if _, err := w.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn = %v, want a quiet ending on a lease lost at release", err)
	}
}

func TestTurn_runMovesUnderTheTurn_staleAppendIsRefusedAndNothingParks(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	q := &fencedQueue{Memory: store, renewed: make(chan struct{}, 1)}
	p := newGatedPlanner()
	w := newFencedWorker(t, Config{Lease: time.Minute}, p, q, store)
	openRun(t, store)

	done := make(chan error, 1)
	go func() {
		_, err := w.turn(context.Background(), slog.New(slog.DiscardHandler))
		done <- err
	}()

	select {
	case <-p.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the turn never reached planning")
	}
	// The run moves while this turn is still planning: a successor worker —
	// one this worker's next renewal would have discovered — already appended
	// its own turn. The suspended worker's append must be refused, because
	// renewal alone cannot fence a worker that acts before its next tick.
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: "run-1", Kind: domain.StepPlanned,
		Scope:   domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", At: time.Now(), Payload: []byte(`{"node":"successor"}`),
	}); err != nil {
		t.Fatalf("successor append: %v", err)
	}
	close(p.release)

	if err := <-done; err != nil {
		t.Fatalf("turn = %v, want a quiet stand-down when the run moved on", err)
	}

	steps, err := store.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// run_started plus the successor's step — and nothing sealed on top by the
	// worker whose decision went stale.
	if len(steps) != 2 {
		t.Fatalf("ledger has %d steps, want 2: the stale turn appended %v", len(steps), steps[2:])
	}
	if got := phaseOf(t, store); got == engine.PhaseParked {
		t.Error("a stale turn parked the run — that is the live run's story, not this worker's")
	}
	// And the lease was handed back clean: going stale is not a failure of the
	// run, so it must be claimable right away with no attempt counted.
	next, err := store.Claim(context.Background(), "w2", time.Minute)
	if err != nil {
		t.Fatalf("Claim after the stand-down = %v, want the run back immediately", err)
	}
	if next.Attempts != 0 {
		t.Errorf("attempts = %d, want 0 — a stale turn is not a failure", next.Attempts)
	}
}

func TestTurn_successorClaimsWhileTheTurnPlans_theStaleWriteIsFenced(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	// Renewals lie: they return nil without extending — the worker suspended
	// between ticks, believing its lease holds while it quietly expires.
	q := &fencedQueue{Memory: store, renewed: make(chan struct{}, 1), renewNoop: true}
	p := newGatedPlanner()
	w := newFencedWorker(t, Config{Lease: 50 * time.Millisecond}, p, q, store)
	openRun(t, store)

	done := make(chan error, 1)
	go func() {
		_, err := w.turn(context.Background(), slog.New(slog.DiscardHandler))
		done <- err
	}()

	select {
	case <-p.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the turn never reached planning")
	}
	// The lease runs out under the planning call, and a successor claims —
	// writing nothing, so the head is exactly where the stale worker left it.
	deadline := time.After(5 * time.Second)
	for {
		if _, err := store.Claim(context.Background(), "w2", time.Minute); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("the successor never managed to claim the expired lease")
		case <-time.After(10 * time.Millisecond):
		}
	}
	close(p.release)

	if err := <-done; err != nil {
		t.Fatalf("turn = %v, want a quiet stand-down behind the fence", err)
	}
	steps, err := store.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("ledger has %d steps, want 1: the stale turn sealed %v", len(steps), steps[1:])
	}
	if got := phaseOf(t, store); got == engine.PhaseParked {
		t.Error("a fenced turn parked the run — that is the successor's story")
	}
}

func TestTurn_renewalCannotProveTheLease_turnIsCancelledBeforeExpiry(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	// Not a lost lease — a renewal that cannot answer at all: the database is
	// away. A worker that keeps acting on a lease nobody can prove holds is
	// the suspended-worker hole with a different mask.
	q := &fencedQueue{Memory: store, renewed: make(chan struct{}, 1), renewErr: errors.New("database unreachable")}
	p := newGatedPlanner() // only a cancel can end the plan
	w := newFencedWorker(t, Config{Lease: 90 * time.Millisecond}, p, q, store)
	openRun(t, store)

	done := make(chan error, 1)
	go func() {
		_, err := w.turn(context.Background(), slog.New(slog.DiscardHandler))
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("turn = %v, want a quiet stand-down", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the turn kept running on a lease it could not prove")
	}
	select {
	case <-p.sawCancel:
	default:
		t.Fatal("the planner was never cancelled")
	}
	if got := q.releases.Load(); got != 0 {
		t.Errorf("releases = %d, want none: the lease may already be someone else's", got)
	}
}

func TestTurn_tinyLease_doesNotPanicTheRenewTicker(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	q := &fencedQueue{Memory: store, renewed: make(chan struct{}, 1)}
	w := newFencedWorker(t, Config{Lease: 2}, &flakyPlanner{}, q, store) // 2ns: Lease/3 truncates to zero
	openRun(t, store)

	if _, err := w.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("turn: %v", err)
	}
}

// blackholeQueue answers claims and releases but its renewals neither return
// nor honour the context — the database is not erroring, it is simply gone.
type blackholeQueue struct {
	*ledger.Memory
	freed chan struct{}
}

func (q *blackholeQueue) Renew(context.Context, domain.RunID, int64, time.Duration) error {
	<-q.freed
	return nil
}

func TestTurn_renewalHangsInsteadOfFailing_turnIsStillCancelledBeforeExpiry(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	q := &blackholeQueue{Memory: store, freed: make(chan struct{})}
	defer close(q.freed)
	p := newGatedPlanner() // only a cancel can end the plan
	w := newFencedWorker(t, Config{Lease: 90 * time.Millisecond}, p, q, store)
	openRun(t, store)

	done := make(chan error, 1)
	go func() {
		_, err := w.turn(context.Background(), slog.New(slog.DiscardHandler))
		done <- err
	}()

	// A renewal that hangs proves nothing, exactly like one that errors. The
	// turn must be cancelled before the lease's edge — not sit inside the
	// blocked call, acting past an expiry nobody extended.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("turn = %v, want a quiet stand-down", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the turn outlived a lease nobody could prove was renewed")
	}
	select {
	case <-p.sawCancel:
	default:
		t.Fatal("the planner was never cancelled")
	}
}

func TestAbandonedFailure_unknownCompensationIsExplicitlyActionable(t *testing.T) {
	t.Parallel()

	got := abandonedFailure(2, 1)
	if got.Code != "abandoned" {
		t.Fatalf("code = %q, want abandoned", got.Code)
	}
	if !strings.Contains(got.Message, "1 compensation outcome(s) unknown") ||
		!strings.Contains(got.Message, "manual reconciliation required") {
		t.Fatalf("message = %q, want the unknown outcome and required action", got.Message)
	}
}

// headHookLedger lets a test act in the gap between the supervisor reading
// the head and writing on top of it — the gap a human's abandonment lands in.
type headHookLedger struct {
	*ledger.Memory
	afterHead func()
}

func (l *headHookLedger) Head(ctx context.Context, runID domain.RunID) (domain.Step, error) {
	s, err := l.Memory.Head(ctx, runID)
	if l.afterHead != nil {
		l.afterHead()
	}
	return s, err
}

func TestPark_humanAbandonsBetweenTheReadAndTheWrite_parkStandsDown(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	hooked := &headHookLedger{Memory: store}
	q := &fencedQueue{Memory: store, renewed: make(chan struct{}, 1)}
	w := newFencedWorker(t, Config{}, &flakyPlanner{}, q, store)
	w.deps.Ledger = hooked
	openRun(t, store)

	claim, err := store.Claim(context.Background(), "w1", time.Minute)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	hooked.afterHead = func() {
		hooked.afterHead = nil
		if _, err := store.Append(context.Background(), domain.Step{
			RunID: "run-1", Kind: domain.StepAbandoned,
			Scope: domain.Scope{Company: "acme", Area: "cx"}, AgentID: "triage",
			At:      time.Now(),
			Payload: []byte(`{"by":"ana","reason":"over","compensate":true}`),
		}); err != nil {
			t.Errorf("abandonment: %v", err)
		}
	}

	if err := w.park(context.Background(), claim, errors.New("gave up"), "attempts_exhausted"); err != nil {
		t.Fatalf("park = %v, want a quiet stand-down over the abandonment", err)
	}

	// The person ended the run; the park must not bury that under a step that
	// blocks the compensation from ever being claimed.
	if got := phaseOf(t, store); got != engine.PhaseCompensating {
		t.Fatalf("phase = %v, want compensating: the park overwrote the abandonment", got)
	}
	if _, err := store.Claim(context.Background(), "w2", time.Minute); err != nil {
		t.Errorf("Claim for the compensation = %v, want the run claimable", err)
	}
}

func TestPark_headAlreadyTerminal_releaseDoesNotRelabelTheRunParked(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	q := &fencedQueue{Memory: store, renewed: make(chan struct{}, 1)}
	w := newFencedWorker(t, Config{}, &flakyPlanner{}, q, store)
	openRun(t, store)

	claim, err := store.Claim(context.Background(), "w1", time.Minute)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	// The runner already ended the run inside this very turn.
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: "run-1", Kind: domain.StepRunFinished,
		Scope: domain.Scope{Company: "acme", Area: "cx"}, AgentID: "triage",
		At: time.Now(), Payload: []byte(`{"reason":"finish_tool"}`),
	}); err != nil {
		t.Fatalf("finish: %v", err)
	}

	if err := w.park(context.Background(), claim, errors.New("late failure"), "attempts_exhausted"); err != nil {
		t.Fatalf("park: %v", err)
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.outcomes) != 1 {
		t.Fatalf("releases = %d, want exactly one", len(q.outcomes))
	}
	// Parked on the outcome is what flips the projection's phase; a finished
	// run relabelled parked reads as work a human still owes.
	if q.outcomes[0].Parked {
		t.Error("a terminal run was released as parked, overwriting its ending in the projection")
	}
}

func TestPark_humanAlreadyAbandoned_parkStandsDownInsteadOfBuryingIt(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	q := &fencedQueue{Memory: store, renewed: make(chan struct{}, 1)}
	w := newFencedWorker(t, Config{}, &flakyPlanner{}, q, store)
	openRun(t, store)

	claim, err := store.Claim(context.Background(), "w1", time.Minute)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	// The person ends the run while the worker's turn is failing — before the
	// supervisor even looks. The abandonment IS the head the supervisor reads,
	// so a head CAS alone accepts it and seals the parking on top.
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: "run-1", Kind: domain.StepAbandoned,
		Scope: domain.Scope{Company: "acme", Area: "cx"}, AgentID: "triage",
		At:      time.Now(),
		Payload: []byte(`{"by":"ana","reason":"over","compensate":true}`),
	}); err != nil {
		t.Fatalf("abandonment: %v", err)
	}

	if err := w.park(context.Background(), claim, errors.New("gave up"), "attempts_exhausted"); err != nil {
		t.Fatalf("park = %v, want a quiet stand-down over the abandonment", err)
	}
	if got := phaseOf(t, store); got != engine.PhaseCompensating {
		t.Fatalf("phase = %v, want compensating: the park buried the person's decision", got)
	}
	if _, err := store.Claim(context.Background(), "w2", time.Minute); err != nil {
		t.Errorf("Claim for the compensation = %v, want the run claimable", err)
	}
}

// hangOnceQueue hangs its first renewal for ever, ignoring the context — and
// answers normally from the second on: a blackhole that recovers.
type hangOnceQueue struct {
	*ledger.Memory
	calls atomic.Int32
	stuck chan struct{}
}

func (q *hangOnceQueue) Renew(ctx context.Context, runID domain.RunID, epoch int64, lease time.Duration) error {
	if q.calls.Add(1) == 1 {
		<-q.stuck
		return nil
	}
	return q.Memory.Renew(ctx, runID, epoch, lease)
}

func TestTurn_oneHungRenewal_doesNotWedgeTheAskerForGood(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	q := &hangOnceQueue{Memory: store, stuck: make(chan struct{})}
	defer close(q.stuck)
	p := newGatedPlanner()
	w := newFencedWorker(t, Config{Lease: 150 * time.Millisecond}, p, q, store)
	openRun(t, store)

	done := make(chan error, 1)
	go func() {
		_, err := w.turn(context.Background(), slog.New(slog.DiscardHandler))
		done <- err
	}()

	// The second tick must still ask: an asker stuck inside the first call
	// means one slow renewal silently disables renewal for the whole turn.
	deadline := time.After(5 * time.Second)
	for q.calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("the asker wedged behind one hung renewal and never asked again")
		case <-time.After(5 * time.Millisecond):
		}
	}
	close(p.release)

	if err := <-done; err != nil {
		t.Fatalf("turn: %v", err)
	}
	select {
	case <-p.sawCancel:
		t.Fatal("the turn was cancelled although the second renewal proved the lease")
	default:
	}
	if got := phaseOf(t, store); got != engine.PhaseFinished {
		t.Errorf("phase = %v, want finished: the recovered renewal should carry the turn", got)
	}
}

func TestConfigWithDefaults_tinyLease_isRaisedToAFloorTheGuardCanBeat(t *testing.T) {
	t.Parallel()

	cfg := Config{Lease: 2 * time.Millisecond}
	cfg.withDefaults()
	// Below the floor, the renew tick bottoms out and the guard fires after
	// the lease has already expired — a watchdog that barks post mortem.
	if cfg.Lease < 30*time.Millisecond {
		t.Fatalf("Lease = %v, want at least the 30ms floor", cfg.Lease)
	}
}

// countingBlackholeQueue hangs every renewal for ever, counting how many were
// ever started: the resource bill of a store that ignores its context.
type countingBlackholeQueue struct {
	*ledger.Memory
	started atomic.Int32
	stuck   chan struct{}
}

func (q *countingBlackholeQueue) Renew(context.Context, domain.RunID, int64, time.Duration) error {
	q.started.Add(1)
	<-q.stuck
	return nil
}

func TestTurn_continuousBlackhole_orphanRenewalsAreBoundedAcrossTurns(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	q := &countingBlackholeQueue{Memory: store, stuck: make(chan struct{})}
	defer close(q.stuck)
	w := newFencedWorker(t, Config{Concurrency: 1, Lease: 40 * time.Millisecond}, nil, q, store)

	// Five turns against a store that never answers. Every started call is a
	// goroutine pinned until the store returns — unbounded, that is a leak
	// that grows with every turn the pool survives.
	for range 5 {
		openRun(t, store)
		p := newGatedPlanner()
		w.deps.Planner = p
		w.specs = staticSpecs{planner: p}

		done := make(chan error, 1)
		go func() {
			_, err := w.turn(context.Background(), slog.New(slog.DiscardHandler))
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("turn: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a turn against the blackhole never ended")
		}
		// Let the lease die so the next turn can claim the run again.
		time.Sleep(60 * time.Millisecond)
	}

	// With one slot per concurrency lane (doubled for headroom), the pinned
	// calls cannot exceed the pool's budget, however many turns run.
	if got := q.started.Load(); got > 2 {
		t.Fatalf("renewals started = %d, want at most 2: every one is a goroutine pinned for ever", got)
	}
}

// A refusal now arrives as a spend carried with the failure (#132). The turn
// records the billed call, and the run then takes exactly the path a refusal
// took before — the spend is new information, not a new outcome.
func TestTurn_refusalWithItsSpend_recordsItAndTakesTheRefusalPath(t *testing.T) {
	t.Parallel()

	refused := func() error {
		return &model.ProviderError{Provider: "anthropic", Code: model.CodeRefused, Err: model.ErrRefused}
	}
	phaseAfter := func(err error) (engine.Phase, []domain.Step) {
		s := newSetup(t, Config{MaxAttempts: 3, BaseBackoff: time.Second}, &flakyPlanner{failures: 1, err: err}, nil)
		openRun(t, s.store)
		if _, terr := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); terr != nil {
			t.Fatalf("turn: %v", terr)
		}
		runs, rerr := s.store.Runs(context.Background())
		if rerr != nil || len(runs) != 1 {
			t.Fatalf("Runs = %v, %v", runs, rerr)
		}
		steps, rerr := s.store.Read(context.Background(), runs[0], domain.FirstSeq)
		if rerr != nil {
			t.Fatalf("Read: %v", rerr)
		}
		return phaseOf(t, s.store), steps
	}

	before, _ := phaseAfter(refused())
	after, steps := phaseAfter(&engine.SpentPlanError{
		Cost:     domain.Cost{InputTokens: 1200, Micros: 42},
		Provider: "anthropic", Model: "claude-opus-5-5",
		Err: refused(),
	})

	if after != before {
		t.Errorf("phase = %v, want %v: carrying the spend must not change where a refusal leads", after, before)
	}
	recorded := 0
	for _, step := range steps {
		if step.Kind == domain.StepPlanned && step.Cost.Micros == 42 {
			recorded++
		}
	}
	if recorded != 1 {
		t.Errorf("spend steps = %d, want the refusal's billed call recorded exactly once", recorded)
	}
}
