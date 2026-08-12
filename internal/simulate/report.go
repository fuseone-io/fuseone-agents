package simulate

import (
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
The report is a fold of the ledger, like every other projection here.

A simulated run writes the same steps a real one writes, so there is nothing to
keep in step: what the Gate decided, what each turn cost and where the run
stopped are already recorded, and this reads them back. A report accumulated
alongside the run would be a second account of the same events, and the two
would disagree the first time anything went wrong.
*/

// Settled is where a case ended.
type Settled string

const (
	SettledFinished Settled = "finished"
	SettledParked   Settled = "parked"
	SettledWaiting  Settled = "awaiting_approval"
	// SettledUnsettled is a run that was still going when the turn bound ran
	// out. It is a result, not a missing row: a planner that will not finish
	// is one of the things a simulation exists to reveal.
	SettledUnsettled Settled = "unsettled"
)

// Act is one thing the agent proposed, and what the Gate did about it.
//
// Anchored on the decision rather than on the call, because the proposals that
// never became calls are the ones an author most needs to see.
type Act struct {
	// Step is the envelope it was proposed in, which is what a correction
	// anchors to (PRD FU-13).
	Step    string         `json:"step,omitempty"`
	Tool    domain.ToolID  `json:"tool"`
	Effect  domain.Effect  `json:"effect"`
	Verdict domain.Verdict `json:"verdict"`
	Rule    string         `json:"rule,omitempty"`
	// Policy names the authored rule, when one decided. Without it every
	// policy decision reads "blocked by policy", which tells an author nothing
	// about what to change and cannot tell two rules apart (AU-10).
	Policy string `json:"policy,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Reached says the proposal got as far as the tool layer — which under
	// simulation is exactly where it stopped.
	Reached bool `json:"reached"`
}

// Case is what happened to one occurrence.
type Case struct {
	RunID   domain.RunID `json:"run_id,omitempty"`
	Settled Settled      `json:"settled"`
	Steps   int          `json:"steps"`
	Cost    domain.Cost  `json:"cost"`
	Acted   []Act        `json:"acted,omitempty"`
	Outcome string       `json:"outcome,omitempty"`
	Reason  string       `json:"reason,omitempty"`
	// Error is set when the case never got a run at all. It is a row rather
	// than an omission: a report that silently drops what it could not run
	// tells an author the set was covered when it was not.
	Error string `json:"error,omitempty"`
}

// Fold turns one simulated run's steps into its row in the report.
func Fold(steps []domain.Step) (Case, error) {
	state, err := engine.Fold(steps)
	if err != nil {
		return Case{}, fmt.Errorf("simulate: fold %d steps: %w", len(steps), err)
	}

	f := folder{c: Case{
		RunID: state.RunID, Settled: settled(state.Phase), Steps: len(steps),
	}}
	for _, step := range steps {
		f.c.Cost = f.c.Cost.Add(step.Cost)
		if err := f.apply(step); err != nil {
			return Case{}, err
		}
	}
	return f.c, nil
}

// folder carries the one thing a step cannot say about itself: which declared
// step of the agent the proposal came from, which the planned step before it
// recorded.
type folder struct {
	c    Case
	node string
}

func (f *folder) apply(step domain.Step) error {
	switch step.Kind {
	case domain.StepPlanned:
		var p domain.PlannedPayload
		if err := decodeInto(step, &p); err != nil {
			return err
		}
		f.node = p.Node

	case domain.StepGateDecided:
		var p domain.GateDecidedPayload
		if err := decodeInto(step, &p); err != nil {
			return err
		}
		f.c.Acted = append(f.c.Acted, Act{
			Step: f.node, Tool: p.Tool, Effect: p.Effect,
			Verdict: p.Verdict, Rule: p.Rule, Policy: p.PolicyCode, Reason: p.Reason,
		})

	case domain.StepToolCalled:
		// The most recent decision is this call's. Matching by tool name would
		// pair a second call with the first decision when a run calls the same
		// tool twice.
		if n := len(f.c.Acted); n > 0 {
			f.c.Acted[n-1].Reached = true
		}

	default:
		return f.ending(step)
	}
	return nil
}

// ending records why the case stopped where it did.
func (f *folder) ending(step domain.Step) error {
	switch step.Kind {
	case domain.StepFailed:
		var p domain.FailedPayload
		if err := decodeInto(step, &p); err != nil {
			return err
		}
		f.c.Reason = p.Code

	case domain.StepParked:
		var p domain.ParkedPayload
		if err := decodeInto(step, &p); err != nil {
			return err
		}
		f.c.Reason = p.Reason

	case domain.StepRunFinished:
		var p domain.RunFinishedPayload
		if err := decodeInto(step, &p); err != nil {
			return err
		}
		f.c.Outcome = p.Outcome
	}
	return nil
}

func settled(phase engine.Phase) Settled {
	switch phase {
	case engine.PhaseFinished:
		return SettledFinished
	case engine.PhaseParked:
		return SettledParked
	case engine.PhaseAwaitingApproval:
		return SettledWaiting
	default:
		return SettledUnsettled
	}
}

func decodeInto(step domain.Step, into any) error {
	if len(step.Payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(step.Payload, into); err != nil {
		return fmt.Errorf("simulate: step %d payload: %w", step.Seq, err)
	}
	return nil
}

// Report is one simulation, ready to be read.
type Report struct {
	ID      string           `json:"id"`
	Agent   domain.AgentID   `json:"agent"`
	Version domain.VersionID `json:"version"`
	Cases   []Case           `json:"cases"`

	// Running is whether any case has yet to settle. Derived rather than
	// tracked: the runs are the queue, so a simulation is still going exactly
	// when one of its runs is, and a flag kept beside them could say otherwise.
	Running bool `json:"running"`
}

// Tally is the line an author reads before any of the rows.
type Tally struct {
	Cases     int `json:"cases"`
	Finished  int `json:"finished"`
	Parked    int `json:"parked"`
	Waiting   int `json:"waiting"`
	Unsettled int `json:"unsettled"`
	// Stopped counts cases the Gate refused at least once, wherever they
	// ended. Counted apart from where a case settled because a run that was
	// refused and carried on is still one somebody has to look at.
	Stopped int `json:"stopped"`
	// NotRun counts cases that never opened a run at all.
	NotRun int         `json:"not_run"`
	Cost   domain.Cost `json:"cost"`
}

// Tally counts the report. Derived rather than accumulated, for the same
// reason the rows are: one source, read twice, cannot disagree with itself.
func (r Report) Tally() Tally {
	t := Tally{Cases: len(r.Cases)}
	for _, c := range r.Cases {
		t.Cost = t.Cost.Add(c.Cost)
		if c.RunID == "" {
			t.NotRun++
			continue
		}
		if stopped(c) {
			t.Stopped++
		}
		switch c.Settled {
		case SettledFinished:
			t.Finished++
		case SettledParked:
			t.Parked++
		case SettledWaiting:
			t.Waiting++
		default:
			t.Unsettled++
		}
	}
	return t
}

func stopped(c Case) bool {
	for _, act := range c.Acted {
		if act.Verdict == domain.VerdictBlock {
			return true
		}
	}
	return false
}
