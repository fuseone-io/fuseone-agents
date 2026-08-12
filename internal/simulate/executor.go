package simulate

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/trigger"
)

// Opener opens a run, declared here by the consumer.
//
// The same opener every other trigger uses. A simulated run is a real run with
// one thing missing, and a second path that "just appends run_started" is how
// the mark, the pinned version or the idempotency key gets forgotten on one of
// them.
type Opener interface {
	Open(ctx context.Context, req trigger.Request) (trigger.Result, error)
}

// defaultTurns bounds one case when a job does not say.
const defaultTurns = 12

// Job is one simulation: one agent version against a set of cases.
type Job struct {
	// ID names this simulation. Generated at the edge and recorded, so the
	// runs it opened can be found again by whoever reads the report.
	ID      string
	Agent   domain.AgentID
	Version domain.VersionID

	// Start is the agent version resolved once, up front: the pack it may
	// reach, the steps it advances through, and its ceiling. Resolved by the
	// caller, because this package must not know how a definition is stored.
	Start   engine.Start
	Planner engine.Planner

	Cases [][]byte
	// MaxTurns bounds one case. Zero takes the default.
	MaxTurns int
}

// Executor runs an agent version against a set of cases.
type Executor struct {
	opener Opener
	deps   engine.Deps
}

/*
NewExecutor is the only place a simulated run's dependencies are built.

Whatever tool layer and planner the caller was holding are dropped here. That
is the property the whole feature rests on: there is no argument, no flag and
no second constructor by which a run marked simulated could reach a real
system.

Everything else is the production collaborator — the same Gate, the same
ledger, the same clock. A simulation that decided differently would answer a
question nobody asked.
*/
func NewExecutor(opener Opener, deps engine.Deps) *Executor {
	deps.Tools = nil
	deps.Planner = nil
	return &Executor{opener: opener, deps: deps}
}

// Run drives every case and folds each run into its row.
//
// One case failing never stops the rest: a set of fifty exists so the odd one
// that cannot run is visible beside the forty-nine that did.
func (e *Executor) Run(ctx context.Context, job Job) (Report, error) {
	switch {
	case e.deps.Ledger == nil:
		return Report{}, errors.New("simulate: the executor has no ledger to read back")
	case job.Planner == nil:
		// Checked before the first run is opened. Falling over on the first
		// turn instead would leave half a simulation in the ledger.
		return Report{}, fmt.Errorf("simulate: %s@%s has nothing to plan with", job.Agent, job.Version)
	}

	report := Report{
		ID: job.ID, Agent: job.Agent, Version: job.Version,
		Cases: make([]Case, 0, len(job.Cases)),
	}
	for i, input := range job.Cases {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		report.Cases = append(report.Cases, e.one(ctx, job, i, input))
	}
	return report, nil
}

// one opens a run for a case, drives it, and reads back what it recorded.
func (e *Executor) one(ctx context.Context, job Job, i int, input []byte) Case {
	opened, err := e.opener.Open(ctx, trigger.Request{
		Agent: job.Agent,
		// The intention is this case of this simulation. Simulating again is a
		// new intention and opens its own runs; retrying one that timed out
		// reaches the runs it already opened.
		IdemKey:    fmt.Sprintf("sim:%s:%d", job.ID, i+1),
		Trigger:    "simulation",
		Input:      input,
		Simulation: job.ID,
	})
	if err != nil {
		return Case{Settled: SettledUnsettled, Error: err.Error()}
	}

	deps := e.deps
	deps.Planner = job.Planner
	deps.Tools = NewDryTools()

	var refused string
	if _, err := Drive(ctx, engine.NewRunner(deps), startFor(job, opened), turnsOf(job)); err != nil {
		refused = err.Error()
	}

	folded, err := e.fold(ctx, opened.RunID)
	if err != nil {
		return Case{RunID: opened.RunID, Settled: SettledUnsettled, Error: err.Error()}
	}
	if refused != "" {
		// Folded first and annotated after: a run that died mid-turn still
		// wrote steps, and they are the most useful part of the row.
		folded.Error = refused
	}
	return folded
}

func startFor(job Job, opened trigger.Result) engine.Start {
	start := job.Start
	start.RunID = opened.RunID
	start.Scope = opened.Scope
	start.AgentID = job.Agent
	start.VersionID = job.Version
	start.Trigger = "simulation"
	return start
}

func (e *Executor) fold(ctx context.Context, runID domain.RunID) (Case, error) {
	steps, err := e.deps.Ledger.Read(ctx, runID, domain.FirstSeq)
	if err != nil {
		return Case{}, fmt.Errorf("simulate: read %s: %w", runID, err)
	}
	return Fold(steps)
}

func turnsOf(job Job) int {
	if job.MaxTurns > 0 {
		return job.MaxTurns
	}
	return defaultTurns
}
