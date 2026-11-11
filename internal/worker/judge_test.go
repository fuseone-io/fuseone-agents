package worker

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A judge run is never planned as the agent it judges (NT-016 § Rollout).

Its input is the case and its version is the judged one, so a pool that resolved it as an
ordinary experiment run would run the judged agent itself on the case and spend money on
an answer nobody asked for. Until this worker can run a judge, it parks one, and it parks
one outside an experiment whatever it can do: a judge is honoured only on a simulated run
that names the experiment it judges.
*/

func judgeOpening() *domain.JudgeOpening {
	return &domain.JudgeOpening{
		Run: "run-exp-1", RubricDigest: "sha256:rubric", Provider: "anthropic", Model: "judge",
		CeilingMicros: 1000, Repetitions: 1, Repetition: 1,
	}
}

func TestTurn_aJudgeRun_isNeverPlannedAsTheJudgedAgent(t *testing.T) {
	t.Parallel()
	config := &domain.ExperimentConfig{Model: "other-model"}
	cases := []struct {
		name    string
		started domain.RunStartedPayload
		real    bool
	}{
		{"a judge run of an experiment", domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "jdg-1", Case: "case-1",
			Experiment: "exp-1", Judge: judgeOpening(),
		}, false},
		// The value the bug takes: a configuration beside the judge would otherwise send the
		// run down the experiment's path and plan the judged agent on another model.
		{"a judge run carrying an experiment's configuration", domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "jdg-1", Case: "case-1",
			Experiment: "exp-1", ExperimentConfig: config, Judge: judgeOpening(),
		}, false},
		{"a judge naming no experiment", domain.RunStartedPayload{
			Trigger: "simulation", Simulated: true, Simulation: "jdg-1", Case: "case-1",
			Judge: judgeOpening(),
		}, false},
		{"a judge on a real run", domain.RunStartedPayload{
			Trigger: "manual", Experiment: "exp-1", Judge: judgeOpening(),
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			openSimulated(t, store, c.started)
			planner := &flakyPlanner{}
			specs := &experimentSpecs{planner: planner}

			if c.real {
				turnOn(t, store, store, specs)
			} else {
				simulationTurn(t, store, specs)
			}

			if specs.declared != 0 || len(specs.asked) != 0 || planner.calls != 0 {
				t.Errorf("resolved the version %d times, the experiment %+v, planned %d times; want none",
					specs.declared, specs.asked, planner.calls)
			}
			if got := parkReason(t, store); got != "spec_unresolved" {
				t.Errorf("reason = %q, want the run parked as unresolved", got)
			}
		})
	}
}

// Which refusal matters once this worker runs judges: one outside an experiment stays
// refused, and only that one.
func TestJudgeRefusal_saysWhetherTheJudgeWasOutsideAnExperiment(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		started domain.RunStartedPayload
		want    error
	}{
		{"of an experiment", domain.RunStartedPayload{Simulated: true, Experiment: "exp-1", Judge: judgeOpening()}, errJudgeNotRunHere},
		{"naming no experiment", domain.RunStartedPayload{Simulated: true, Judge: judgeOpening()}, errJudgeOutsideExperiment},
		{"on a real run", domain.RunStartedPayload{Experiment: "exp-1", Judge: judgeOpening()}, errJudgeOutsideExperiment},
	} {
		if err := judgeRefusal("run-1", c.started); !errors.Is(err, c.want) {
			t.Errorf("%s: refusal = %v, want %v", c.name, err, c.want)
		}
	}
}

// countingTools is a real pool's tool layer that only counts what reached it.
type countingTools struct{ reserved, invoked int }

func (c *countingTools) Reserve(context.Context, Call) error { c.reserved++; return nil }

func (c *countingTools) Invoke(context.Context, Call) (engine.ToolResult, error) {
	c.invoked++
	return engine.ToolResult{}, nil
}

// realPoolTurn runs one turn of the pool that advances real runs, wired as production
// wires it: no experiments, real tools.
func realPoolTurn(t *testing.T, store *ledger.Memory, specs *experimentSpecs, tools *countingTools) {
	t.Helper()
	clock := frozenClock{t: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	deps := engine.Deps{
		Ledger: store, Content: engine.NewMemoryContent(), Gate: gate.New(),
		Tools: tools, Catalog: emptyCatalog{}, Clock: clock,
	}
	w := New(Config{Owner: "w-real"}, store, deps, specs, clock, slog.New(slog.DiscardHandler)).
		WithStages(stageIs(domain.StageAutonomous))
	if claimed, err := w.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil || !claimed {
		t.Fatalf("turn: claimed=%v err=%v", claimed, err)
	}
}

/*
The pool that advances real runs reads no experiment, and still never plans a judge run.

A run that is not simulated and names a judge is the one a forged or buggy opener would
make, and the real pool is the one that claims it. Resolved there as the version it
judges, it would plan the judged agent with real tools. The anchor is the same run with
no judge: it is resolved, planned, and its proposal reaches the tool layer, so the
absences below are the fence's.
*/
func TestTurn_aJudgeOnARealRun_inThePoolWithoutExperiments_neverReachesATool(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		judge *domain.JudgeOpening
	}{{"with a judge", judgeOpening()}, {"anchor, without one", nil}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			openSimulated(t, store, domain.RunStartedPayload{
				Trigger: "manual", Experiment: "exp-1", Judge: c.judge,
			})
			planner := &flakyPlanner{proposal: &engine.Proposal{Tool: "crm.lookup", Args: []byte(`{}`)}}
			specs := &experimentSpecs{planner: planner}
			tools := &countingTools{}

			realPoolTurn(t, store, specs, tools)

			reached := specs.declared + planner.calls + tools.reserved + tools.invoked
			if c.judge == nil {
				if specs.declared == 0 || planner.calls == 0 || tools.invoked == 0 {
					t.Fatalf("anchor: resolved %d, planned %d, invoked %d; the test cannot see a tool reached",
						specs.declared, planner.calls, tools.invoked)
				}
				return
			}
			if reached != 0 {
				t.Errorf("resolved %d, planned %d, reserved %d, invoked %d; want none",
					specs.declared, planner.calls, tools.reserved, tools.invoked)
			}
			if got := parkReason(t, store); got != "spec_unresolved" {
				t.Errorf("reason = %q, want the run parked as unresolved", got)
			}
		})
	}
}

// readCounting is a ledger that counts how the fence read the run.
type readCounting struct {
	*ledger.Memory
	reads, openings int
}

func (r *readCounting) Read(ctx context.Context, run domain.RunID, from int64) ([]domain.Step, error) {
	r.reads++
	return r.Memory.Read(ctx, run, from)
}

func (r *readCounting) Opening(ctx context.Context, run domain.RunID) (domain.Step, error) {
	r.openings++
	return r.Memory.Opening(ctx, run)
}

/*
The fence costs one step, not the run (NT-016 § Rollout).

Every turn of the real pool reads the opening now. Read whole, a long run is a scan that
grows with it on every turn, on top of the runner's own two. A judge run never reaches the
runner, so every read made here is the fence's: it must be the opening alone, in both
pools, however many steps the run already has.
*/
func TestTurn_theJudgeFence_readsTheOpeningAlone(t *testing.T) {
	t.Parallel()
	for _, simulated := range []bool{true, false} {
		name := map[bool]string{true: "simulation pool", false: "real pool"}[simulated]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := ledger.NewMemory()
			openSimulated(t, store, domain.RunStartedPayload{
				Trigger: "simulation", Simulated: simulated, Simulation: "jdg-1", Case: "case-1",
				Experiment: "exp-1", Judge: judgeOpening(),
			})
			for i := 0; i < 40; i++ {
				if _, err := store.Append(context.Background(), domain.Step{
					RunID: "run-1", Kind: domain.StepPlanned, Scope: domain.Scope{Company: "acme", Area: "cx"},
					AgentID: "triage", VersionID: "v3", At: time.Now(), Payload: []byte(`{"node":"n"}`),
				}); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
			counted := &readCounting{Memory: store}
			specs := &experimentSpecs{planner: &flakyPlanner{}}
			clock := frozenClock{t: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
			deps := engine.Deps{
				Ledger: counted, Content: engine.NewMemoryContent(), Gate: gate.New(),
				Tools: &countingTools{}, Catalog: emptyCatalog{}, Clock: clock,
			}
			var queue Queue = store
			if simulated {
				queue = store.Simulations()
			}
			w := New(Config{Owner: "w"}, queue, deps, specs, clock, slog.New(slog.DiscardHandler)).
				WithStages(stageIs(domain.StageAutonomous))
			if simulated {
				w = w.WithExperiments(specs)
			}
			if claimed, err := w.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil || !claimed {
				t.Fatalf("turn: claimed=%v err=%v", claimed, err)
			}

			if counted.openings != 1 || counted.reads != 0 {
				t.Errorf("read the opening %d times and the whole run %d times; want the opening once and nothing else",
					counted.openings, counted.reads)
			}
			if got := parkReason(t, store); got != "spec_unresolved" {
				t.Errorf("reason = %q, want the judge run parked", got)
			}
		})
	}
}

// openingless is a ledger that offers no opening read: only the engine's port.
type openingless struct{ engine.Ledger }

// Without a cheap opening read, the fence fails loudly rather than reading the whole run
// every turn or letting the run through unchecked.
func TestTurn_aLedgerThatCannotReadAnOpening_parksRatherThanSkippingTheFence(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	openSimulated(t, store, domain.RunStartedPayload{Trigger: "manual", Judge: judgeOpening()})
	planner := &flakyPlanner{}
	specs := &experimentSpecs{planner: planner}
	clock := frozenClock{t: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	deps := engine.Deps{
		Ledger: openingless{store}, Content: engine.NewMemoryContent(), Gate: gate.New(),
		Tools: &countingTools{}, Catalog: emptyCatalog{}, Clock: clock,
	}
	w := New(Config{Owner: "w"}, store, deps, specs, clock, slog.New(slog.DiscardHandler)).
		WithStages(stageIs(domain.StageAutonomous))
	if claimed, err := w.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil || !claimed {
		t.Fatalf("turn: claimed=%v err=%v", claimed, err)
	}
	if specs.declared != 0 || planner.calls != 0 {
		t.Errorf("resolved %d and planned %d times; want neither", specs.declared, planner.calls)
	}
	if got := parkReason(t, store); got != "spec_unresolved" {
		t.Errorf("reason = %q, want the run parked", got)
	}
}

// recordingJudges is a simulation pool's judge resolver that records what it was asked.
type recordingJudges struct {
	asked   []domain.RunID
	planner *flakyPlanner
}

func (j *recordingJudges) ResolveJudge(
	_ context.Context, run domain.RunID, _ domain.RunStartedPayload,
) (Resolution, error) {
	j.asked = append(j.asked, run)
	return Resolution{Start: engine.Start{Pack: gate.NewPack(), Budget: domain.Budget{Micros: 1000}}, Planner: j.planner}, nil
}

func judgedTurn(t *testing.T, store *ledger.Memory, specs *experimentSpecs, judges *recordingJudges) {
	t.Helper()
	clock := frozenClock{t: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	deps := engine.Deps{
		Ledger: store, Content: engine.NewMemoryContent(), Gate: gate.New(),
		Tools: noTools{}, Catalog: emptyCatalog{}, Clock: clock,
	}
	w := New(Config{Owner: "w-sim"}, store.Simulations(), deps, specs, clock, slog.New(slog.DiscardHandler)).
		WithStages(stageIs(domain.StageDraft)).WithExperiments(specs).WithJudges(judges)
	if claimed, err := w.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil || !claimed {
		t.Fatalf("turn: claimed=%v err=%v", claimed, err)
	}
}

// With judges wired, a judge run of an experiment is resolved as a judge and nothing else.
func TestTurn_aJudgeRun_withJudgesWired_resolvesAsAJudge(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	openSimulated(t, store, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "jdg-1", Case: "case-1",
		Experiment: "exp-1", ExperimentConfig: &domain.ExperimentConfig{Model: "other"}, Judge: judgeOpening(),
	})
	specs := &experimentSpecs{planner: &flakyPlanner{}}
	judges := &recordingJudges{planner: &flakyPlanner{}}

	judgedTurn(t, store, specs, judges)

	if len(judges.asked) != 1 || judges.asked[0] != "run-1" || specs.declared != 0 || len(specs.asked) != 0 {
		t.Errorf("judges asked %v, version %d, experiment %+v; want the judge alone", judges.asked, specs.declared, specs.asked)
	}
	if judges.planner.calls != 1 {
		t.Errorf("the judge planned %d times, want once", judges.planner.calls)
	}
}

// Wiring judges does not widen the fence: a judge naming no experiment stays refused.
func TestTurn_aJudgeOutsideAnExperiment_withJudgesWired_isStillRefused(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	openSimulated(t, store, domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "jdg-1", Case: "case-1", Judge: judgeOpening(),
	})
	specs := &experimentSpecs{planner: &flakyPlanner{}}
	judges := &recordingJudges{planner: &flakyPlanner{}}

	judgedTurn(t, store, specs, judges)

	if len(judges.asked) != 0 || specs.declared != 0 || judges.planner.calls != 0 {
		t.Errorf("judges asked %v, version %d, planned %d; want nothing", judges.asked, specs.declared, judges.planner.calls)
	}
	if got := parkReason(t, store); got != "spec_unresolved" {
		t.Errorf("reason = %q, want the run parked", got)
	}
}
