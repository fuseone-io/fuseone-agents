package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A run opened on a candidate is pinned to the seal its opening names (NT-018 phase 2).

The worker honours the seal only on a simulated run of an experiment or a rehearsal, and
only in a pool that can read seals. Anywhere else, and whenever the seal cannot be read,
the run ends with a failed step carrying fuseone:candidate-unreadable and plans nothing:
never the published version, never another seal of the same version.
*/

const testSeal = "0b7c3e52-8a43-4b55-9a3f-0d6a3c5f9e11"

type recordingCandidates struct {
	asked   []string
	planner *flakyPlanner
	err     error
}

func (c *recordingCandidates) ResolveCandidate(
	_ context.Context, _ domain.Claim, started domain.RunStartedPayload,
) (Resolution, error) {
	c.asked = append(c.asked, started.SealID)
	if c.err != nil {
		return Resolution{}, c.err
	}
	return Resolution{
		Start:   engine.Start{Pack: gate.NewPack(), Budget: domain.Budget{Micros: 1000, Steps: 10}},
		Planner: c.planner,
	}, nil
}

// candidateTurn runs one turn claiming from queue, with candidates wired when given.
func candidateTurn(t *testing.T, store *ledger.Memory, queue Queue, specs *experimentSpecs, candidates Candidates) {
	t.Helper()
	clock := frozenClock{t: time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)}
	deps := engine.Deps{
		Ledger: store, Content: engine.NewMemoryContent(), Gate: gate.New(),
		Tools: noTools{}, Catalog: emptyCatalog{}, Clock: clock,
	}
	w := New(Config{Owner: "w-sim"}, queue, deps, specs, clock, slog.New(slog.DiscardHandler)).
		WithStages(stageIs(domain.StageDraft)).WithExperiments(specs)
	if candidates != nil {
		w = w.WithCandidates(candidates)
	}
	if claimed, err := w.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil || !claimed {
		t.Fatalf("turn: claimed=%v err=%v", claimed, err)
	}
}

func failedCode(t *testing.T, store *ledger.Memory) string {
	t.Helper()
	head, err := store.Head(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if head.Kind != domain.StepFailed {
		t.Fatalf("head = %s, want a failed step", head.Kind)
	}
	var failed domain.FailedPayload
	if err := json.Unmarshal(head.Payload, &failed); err != nil {
		t.Fatalf("decode failure: %v", err)
	}
	return failed.Code
}

func TestTurn_aSealOutsideACandidateRun_failsBeforeThePlanner(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		started domain.RunStartedPayload
		real    bool
	}{
		{"a real run naming a seal", domain.RunStartedPayload{Trigger: "manual", SealID: testSeal}, true},
		// The value the bug takes: simulated, so it reaches the pool that reads seals, but a
		// battery run, neither an experiment nor a rehearsal.
		{"a battery run naming a seal", domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "bat-1", Case: "case-1", SealID: testSeal,
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			openSimulated(t, store, c.started)
			planner := &flakyPlanner{}
			specs := &experimentSpecs{planner: planner}
			candidates := &recordingCandidates{planner: planner}

			queue := Queue(store.Simulations())
			if c.real {
				queue = store
			}
			candidateTurn(t, store, queue, specs, candidates)

			if specs.declared != 0 || len(specs.asked) != 0 || len(candidates.asked) != 0 || planner.calls != 0 {
				t.Errorf("version %d, experiment %+v, candidates %v, planned %d; want none",
					specs.declared, specs.asked, candidates.asked, planner.calls)
			}
			if got := failedCode(t, store); got != CodeCandidateUnreadable {
				t.Errorf("code = %q, want %q", got, CodeCandidateUnreadable)
			}
		})
	}
}

func TestTurn_aCandidateRun_inAPoolThatCannotReadSeals_failsClosed(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	openSimulated(t, store, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "exp-1", Case: "case-1",
		Experiment: "exp-1", SealID: testSeal,
	})
	planner := &flakyPlanner{}
	specs := &experimentSpecs{planner: planner}

	candidateTurn(t, store, store.Simulations(), specs, nil)

	if specs.declared != 0 || planner.calls != 0 {
		t.Errorf("resolved the published version %d times, planned %d; want neither", specs.declared, planner.calls)
	}
	if got := failedCode(t, store); got != CodeCandidateUnreadable {
		t.Errorf("code = %q, want %q", got, CodeCandidateUnreadable)
	}
}

func TestTurn_aCandidateRun_resolvesTheSealItOpenedWith(t *testing.T) {
	t.Parallel()
	for name, started := range map[string]domain.RunStartedPayload{
		"an experiment": {
			Trigger: "simulation", Simulated: true, Simulation: "exp-1", Case: "case-1",
			Experiment: "exp-1", ExperimentConfig: &domain.ExperimentConfig{Model: "other"}, SealID: testSeal,
		},
		"a rehearsal": {
			Trigger: "rehearsal", Simulated: true, Simulation: "reh-1", Rehearsal: true, SealID: testSeal,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			openSimulated(t, store, started)
			specs := &experimentSpecs{planner: &flakyPlanner{}}
			candidates := &recordingCandidates{planner: &flakyPlanner{}}

			candidateTurn(t, store, store.Simulations(), specs, candidates)

			if len(candidates.asked) != 1 || candidates.asked[0] != testSeal ||
				specs.declared != 0 || len(specs.asked) != 0 {
				t.Errorf("candidates %v, version %d, experiment %+v; want the seal alone",
					candidates.asked, specs.declared, specs.asked)
			}
			if candidates.planner.calls != 1 {
				t.Errorf("the candidate planned %d times, want once", candidates.planner.calls)
			}
		})
	}
}

func TestTurn_anUnreadableCandidate_failsWithoutPlanning(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	openSimulated(t, store, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "exp-1", Case: "case-1",
		Experiment: "exp-1", SealID: testSeal,
	})
	specs := &experimentSpecs{planner: &flakyPlanner{}}
	candidates := &recordingCandidates{
		planner: &flakyPlanner{},
		err:     fmt.Errorf("seal %s: %w", testSeal, domain.ErrCandidateUnreadable),
	}

	candidateTurn(t, store, store.Simulations(), specs, candidates)

	if candidates.planner.calls != 0 || specs.declared != 0 {
		t.Errorf("planned %d times, published version %d; want neither", candidates.planner.calls, specs.declared)
	}
	if got := failedCode(t, store); got != CodeCandidateUnreadable {
		t.Errorf("code = %q, want %q", got, CodeCandidateUnreadable)
	}
}

// racingLedger is a ledger where something else lands on the run between the head the
// failure was decided against and its append: a person's abandonment, most importantly.
type racingLedger struct {
	*ledger.Memory
	err error
}

func (l racingLedger) AppendIfLeaseAndHead(
	ctx context.Context, epoch int64, head domain.StepRef, s domain.Step,
) (domain.Step, error) {
	if s.Kind == domain.StepFailed {
		return domain.Step{}, l.err
	}
	return l.Memory.AppendIfLeaseAndHead(ctx, epoch, head, s)
}

// releasingQueue records every release, so a test can tell a lease handed back from one
// left to expire.
type releasingQueue struct {
	Queue
	released []domain.RunID
	outcomes []domain.ClaimOutcome
}

func (q *releasingQueue) Release(ctx context.Context, run domain.RunID, outcome domain.ClaimOutcome) error {
	q.released = append(q.released, run)
	q.outcomes = append(q.outcomes, outcome)
	return q.Queue.Release(ctx, run, outcome)
}

// A failure that loses its race hands the lease back, as a parking does: only a lost lease
// leaves it to the successor. Left leased, the run waits out the whole lease before the
// abandonment that moved it can be compensated.
func TestTurn_aCandidateFailureThatLosesItsRace_releasesTheRunUnlessTheLeaseIsGone(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		err         error
		wantRelease bool
	}{
		{"the head moved", domain.ErrHeadMoved, true},
		{"the store failed", errors.New("connection reset"), true},
		{"the lease was lost", domain.ErrLeaseLost, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			openSimulated(t, store, domain.RunStartedPayload{
				Trigger: "simulation", Simulated: true, Simulation: "exp-1", Case: "case-1",
				Experiment: "exp-1", SealID: testSeal,
			})
			queue := &releasingQueue{Queue: store.Simulations()}
			clock := frozenClock{t: time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)}
			deps := engine.Deps{
				Ledger: racingLedger{Memory: store, err: c.err}, Content: engine.NewMemoryContent(),
				Gate: gate.New(), Tools: noTools{}, Catalog: emptyCatalog{}, Clock: clock,
			}
			specs := &experimentSpecs{planner: &flakyPlanner{}}
			w := New(Config{Owner: "w-sim"}, queue, deps, specs, clock, slog.New(slog.DiscardHandler)).
				WithStages(stageIs(domain.StageDraft)).WithExperiments(specs)

			if _, err := w.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
				t.Fatalf("turn: %v", err)
			}

			if released := len(queue.released) == 1; released != c.wantRelease {
				t.Errorf("released %v, want released=%v", queue.released, c.wantRelease)
			}
		})
	}
}

// A failure that cannot be written goes through the same supervision as any other fault:
// backed off, not reclaimed at once, and parked once the attempts run out. Released with no
// delay and never counted, a store that keeps refusing the write would be retried in a
// tight loop for ever.
func TestTurn_aCandidateFailureThatCannotBeWritten_backsOffAndParksWhenExhausted(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	turnWith := func(t *testing.T, maxAttempts int) (*ledger.Memory, *releasingQueue) {
		t.Helper()
		store := ledger.NewMemory()
		openSimulated(t, store, domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "exp-1", Case: "case-1",
			Experiment: "exp-1", SealID: testSeal,
		})
		queue := &releasingQueue{Queue: store.Simulations()}
		clock := frozenClock{t: now}
		deps := engine.Deps{
			Ledger:  racingLedger{Memory: store, err: errors.New("connection reset")},
			Content: engine.NewMemoryContent(), Gate: gate.New(), Tools: noTools{},
			Catalog: emptyCatalog{}, Clock: clock,
		}
		specs := &experimentSpecs{planner: &flakyPlanner{}}
		w := New(Config{Owner: "w-sim", MaxAttempts: maxAttempts}, queue, deps, specs, clock,
			slog.New(slog.DiscardHandler)).WithStages(stageIs(domain.StageDraft)).WithExperiments(specs)
		if _, err := w.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
			t.Fatalf("turn: %v", err)
		}
		if len(queue.outcomes) != 1 {
			t.Fatalf("released %d times, want once", len(queue.outcomes))
		}
		return store, queue
	}

	t.Run("an attempt left backs off", func(t *testing.T) {
		t.Parallel()
		_, queue := turnWith(t, 5)
		got := queue.outcomes[0]
		if got.Parked || !got.NextAttemptAt.After(now) || got.Err == nil {
			t.Errorf("outcome parked=%v next=%v err=%v; want a counted failure retried later",
				got.Parked, got.NextAttemptAt, got.Err)
		}
	})
	t.Run("the last attempt parks", func(t *testing.T) {
		t.Parallel()
		store, queue := turnWith(t, 1)
		if !queue.outcomes[0].Parked {
			t.Errorf("outcome %+v, want the run parked", queue.outcomes[0])
		}
		if got := parkReason(t, store); got != "attempts_exhausted" {
			t.Errorf("parked as %q, want attempts_exhausted", got)
		}
	})
}
