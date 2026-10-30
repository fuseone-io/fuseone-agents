package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
)

// A provider that keeps failing for reasons a retry is meant to outlast is
// where the declared fallback takes over — for a draft or copilot run, after
// the normal attempts, and for the rest of the run (#27).

var (
	declaredPair = domain.ModelPair{Provider: "anthropic", Model: "claude-opus-5"}
	backupPair   = domain.ModelPair{Provider: "openai", Model: "gpt-test"}
)

type stageIs domain.Stage

func (s stageIs) StageOf(context.Context, domain.AgentID) (domain.Stage, error) {
	return domain.Stage(s), nil
}

// fallbackSpecs resolves a version that declares a fallback, or none.
type fallbackSpecs struct {
	primary, backup engine.Planner
	declared        bool
}

func (s fallbackSpecs) Resolve(context.Context, domain.AgentID, domain.VersionID) (Resolution, error) {
	out := Resolution{
		Start: engine.Start{
			Pack:    gate.NewPack("crm.lookup"),
			Budget:  domain.Budget{Micros: 1_000_000, Steps: 50},
			Trigger: "worker",
		},
		Planner: s.primary,
	}
	if s.declared {
		out.Fallback = &Fallback{From: declaredPair, To: backupPair, Planner: s.backup}
	}
	return out, nil
}

func overloaded() error {
	return &model.ProviderError{
		Provider: "anthropic", Code: model.CodeProviderOverloaded, Status: 529, Retryable: true,
	}
}

type fallbackRig struct {
	store           *ledger.Memory
	primary, backup *flakyPlanner
	specs           fallbackSpecs
	stage           domain.Stage
}

func newFallbackRig(t *testing.T, stage domain.Stage, primaryErr error, declared bool) *fallbackRig {
	t.Helper()
	r := &fallbackRig{
		store:   ledger.NewMemory(),
		primary: &flakyPlanner{failures: 99, err: primaryErr},
		backup:  &flakyPlanner{},
		stage:   stage,
	}
	r.specs = fallbackSpecs{primary: r.primary, backup: r.backup, declared: declared}
	openRun(t, r.store)
	return r
}

// worker is a fresh process: it holds nothing about the run but the ledger.
func (r *fallbackRig) worker(specs Specs) *Worker {
	clock := frozenClock{t: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)}
	deps := engine.Deps{
		Ledger: r.store, Content: engine.NewMemoryContent(), Gate: gate.New(),
		Tools: noTools{}, Catalog: emptyCatalog{}, Clock: clock,
	}
	// Backoff in the past, so every retry is claimable at once.
	cfg := Config{Owner: "w", MaxAttempts: 3, BaseBackoff: -time.Hour}
	return New(cfg, r.store, deps, specs, clock, slog.New(slog.DiscardHandler)).
		WithStages(stageIs(r.stage))
}

func (r *fallbackRig) turns(t *testing.T, w *Worker, n int) {
	t.Helper()
	for range n {
		if _, err := w.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
			t.Fatalf("turn: %v", err)
		}
	}
}

func (r *fallbackRig) steps(t *testing.T) []domain.Step {
	t.Helper()
	steps, err := r.store.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return steps
}

func (r *fallbackRig) switches(t *testing.T) []domain.ModelFallbackPayload {
	t.Helper()
	var out []domain.ModelFallbackPayload
	for _, s := range r.steps(t) {
		if s.Kind != domain.StepModelFallback {
			continue
		}
		var p domain.ModelFallbackPayload
		if err := json.Unmarshal(s.Payload, &p); err != nil {
			t.Fatalf("payload: %v", err)
		}
		out = append(out, p)
	}
	return out
}

func TestTurn_retryableFailureOnACopilotRun_switchesOnlyAfterTheAttempts(t *testing.T) {
	t.Parallel()

	r := newFallbackRig(t, domain.StageCopilot, overloaded(), true)
	w := r.worker(r.specs)

	r.turns(t, w, 2)
	if got := r.switches(t); len(got) != 0 {
		t.Fatalf("switched after 2 of 3 attempts: %+v", got)
	}
	r.turns(t, w, 1)
	if got := r.switches(t); len(got) != 1 {
		t.Fatalf("switches = %+v, want one once the attempts are exhausted", got)
	}
	if phaseOf(t, r.store) == engine.PhaseParked {
		t.Error("the run parked although it had a fallback to switch to")
	}
}

func TestTurn_afterTheSwitch_theNextPlanningCallUsesTheFallback(t *testing.T) {
	t.Parallel()

	r := newFallbackRig(t, domain.StageCopilot, overloaded(), true)
	w := r.worker(r.specs)
	r.turns(t, w, 4)

	if r.primary.calls != 3 {
		t.Errorf("declared pair asked %d times, want the 3 attempts and no more", r.primary.calls)
	}
	if r.backup.calls != 1 {
		t.Errorf("fallback asked %d times, want 1", r.backup.calls)
	}
	if got := phaseOf(t, r.store); got != engine.PhaseFinished {
		t.Errorf("Phase = %v, want the fallback to have finished the run", got)
	}
}

func TestTurn_theSwitchSurvivesADifferentWorkerResumingTheRun(t *testing.T) {
	t.Parallel()

	r := newFallbackRig(t, domain.StageCopilot, overloaded(), true)
	r.turns(t, r.worker(r.specs), 3)

	// Another process, with planners of its own and no memory of the outage.
	// Only the ledger can tell it the run already switched.
	primary, backup := &flakyPlanner{failures: 99, err: overloaded()}, &flakyPlanner{}
	r.turns(t, r.worker(fallbackSpecs{primary: primary, backup: backup, declared: true}), 1)

	if primary.calls != 0 {
		t.Errorf("the resuming worker asked the declared pair %d times", primary.calls)
	}
	if backup.calls != 1 {
		t.Errorf("the resuming worker asked the fallback %d times, want 1", backup.calls)
	}
}

func TestTurn_theSwitch_recordsFromToAndTheFailure(t *testing.T) {
	t.Parallel()

	r := newFallbackRig(t, domain.StageDraft, overloaded(), true)
	r.turns(t, r.worker(r.specs), 3)

	got := r.switches(t)
	if len(got) != 1 {
		t.Fatalf("switches = %+v", got)
	}
	if got[0].From != declaredPair || got[0].To != backupPair {
		t.Errorf("switch = %+v → %+v, want %+v → %+v", got[0].From, got[0].To, declaredPair, backupPair)
	}
	if got[0].Failure.Code != model.CodeProviderOverloaded || got[0].Failure.Provider != "anthropic" ||
		got[0].Failure.Status != 529 {
		t.Errorf("failure = %+v, want the stable summary of what exhausted the attempts", got[0].Failure)
	}
	if got[0].Attempts != 3 {
		t.Errorf("attempts = %d, want 3", got[0].Attempts)
	}
}

func TestTurn_failuresARetryCannotOutlast_neverSwitch(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
	}{
		{"auth", &model.ProviderError{Provider: "anthropic", Code: model.CodeAuthFailed, Status: 401}},
		{"bad request", &model.ProviderError{Provider: "anthropic", Code: model.CodeBadRequest, Status: 400}},
		// Retryable, and nothing to do with the provider: another run holds
		// the effect. Switching vendors cannot release it.
		{"dedupe in flight", engine.DedupeInFlightError{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newFallbackRig(t, domain.StageCopilot, tc.err, true)
			r.turns(t, r.worker(r.specs), 3)

			if got := r.switches(t); len(got) != 0 {
				t.Errorf("switched on %s: %+v", tc.name, got)
			}
			if r.backup.calls != 0 {
				t.Errorf("fallback asked %d times", r.backup.calls)
			}
			if got := phaseOf(t, r.store); got != engine.PhaseParked {
				t.Errorf("Phase = %v, want parked as today", got)
			}
		})
	}
}

func TestTurn_anAutonomousRun_parksAsUnrehearsedAndNeverAsksTheFallback(t *testing.T) {
	t.Parallel()

	r := newFallbackRig(t, domain.StageAutonomous, overloaded(), true)
	r.turns(t, r.worker(r.specs), 4)

	if r.backup.calls != 0 {
		t.Errorf("fallback asked %d times by an autonomous run", r.backup.calls)
	}
	if got := r.switches(t); len(got) != 0 {
		t.Errorf("switches = %+v", got)
	}
	p := parkPayload(t, r.store)
	if p.Reason != domain.ParkFallbackUnrehearsed {
		t.Errorf("reason = %q, want %q", p.Reason, domain.ParkFallbackUnrehearsed)
	}
	if p.Failure == nil || p.Failure.Code != model.CodeProviderOverloaded {
		t.Errorf("failure = %+v, want the provider's own summary kept beside the reason", p.Failure)
	}
}

func TestTurn_noFallbackDeclared_parksExactlyAsBefore(t *testing.T) {
	t.Parallel()

	for _, stage := range []domain.Stage{domain.StageCopilot, domain.StageAutonomous} {
		t.Run(string(stage), func(t *testing.T) {
			t.Parallel()
			r := newFallbackRig(t, stage, overloaded(), false)
			r.turns(t, r.worker(r.specs), 3)

			if got := r.switches(t); len(got) != 0 {
				t.Errorf("switches = %+v", got)
			}
			if p := parkPayload(t, r.store); p.Reason != model.CodeProviderOverloaded || p.Attempts != 3 {
				t.Errorf("parked = %+v, want the provider's code after 3 attempts", p)
			}
		})
	}
}

func TestTurn_theFallbackFailingToo_parksWithoutSwitchingAgain(t *testing.T) {
	t.Parallel()

	r := newFallbackRig(t, domain.StageCopilot, overloaded(), true)
	r.backup.failures = 99
	r.backup.err = &model.ProviderError{Provider: "openai", Code: model.CodeRateLimited, Status: 429, Retryable: true}
	r.turns(t, r.worker(r.specs), 6)

	if got := r.switches(t); len(got) != 1 {
		t.Errorf("switches = %+v, want exactly one: sticky means no switching back", got)
	}
	if r.backup.calls != 3 {
		t.Errorf("fallback asked %d times, want its own full attempts", r.backup.calls)
	}
	if p := parkPayload(t, r.store); p.Reason != model.CodeRateLimited {
		t.Errorf("reason = %q, want the fallback's failure", p.Reason)
	}
}

func TestTurn_promotedToAutonomousAfterTheSwitch_parksAndNeverAsksTheFallback(t *testing.T) {
	t.Parallel()

	// The guard at the moment of the switch is not enough: the switch is
	// sticky and the stage is not. An agent promoted after its run fell back
	// would otherwise act alone on a pair nobody rehearsed it against.
	r := newFallbackRig(t, domain.StageCopilot, overloaded(), true)
	r.turns(t, r.worker(r.specs), 3)
	if len(r.switches(t)) != 1 {
		t.Fatalf("setup: the copilot run did not switch")
	}

	r.stage = domain.StageAutonomous
	r.turns(t, r.worker(r.specs), 1)

	if r.backup.calls != 0 {
		t.Errorf("fallback asked %d times after the agent became autonomous", r.backup.calls)
	}
	if p := parkPayload(t, r.store); p.Reason != domain.ParkFallbackUnrehearsed {
		t.Errorf("reason = %q, want %q", p.Reason, domain.ParkFallbackUnrehearsed)
	}
}
