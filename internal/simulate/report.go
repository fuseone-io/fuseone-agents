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
	// SettledErased is a case that could not run because its input is gone.
	// Nothing is still going, and it neither held nor broke.
	SettledErased Settled = "erased"
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
	// datasetVersion is the pin this run recorded. The report says it, once.
	datasetVersion int64
	// others are the other runs of this case, when it has more than one. Not part of
	// what this row is judged on, and part of what the battery cost and what stopped it.
	others []Case
	// didNotRun marks the row a battery writes for a case of its corpus that opened no
	// run: broken, and costing nothing because nothing ran, which a comparison must not
	// read as cheaper.
	didNotRun bool

	// ID is the regression case this row replays, when it replays one.
	ID      string       `json:"id,omitempty"`
	RunID   domain.RunID `json:"run_id,omitempty"`
	Settled Settled      `json:"settled"`
	Steps   int          `json:"steps"`
	// Model and Effort are what answered, so a report can say whether a case
	// broke against the same model it last held against.
	Model   string      `json:"model,omitempty"`
	Effort  string      `json:"effort,omitempty"`
	Cost    domain.Cost `json:"cost"`
	Acted   []Act       `json:"acted,omitempty"`
	Outcome string      `json:"outcome,omitempty"`
	// OutcomeRef is where the answer lives for a run recorded since it moved to
	// the content store. Fold is pure — no context, no store — so it carries
	// the reference out and whoever has both resolves it.
	OutcomeRef string `json:"outcomeRef,omitempty"`
	Reason     string `json:"reason,omitempty"`
	// Error is set when the case never got a run at all. It is a row rather
	// than an omission: a report that silently drops what it could not run
	// tells an author the set was covered when it was not.
	Error string `json:"error,omitempty"`

	// Expected is what a correction said must be true of this case, and Unmet
	// is what stopped being true. Both empty for an ordinary simulation.
	Expected []domain.Expectation `json:"expected,omitempty"`
	Unmet    []domain.Expectation `json:"unmet,omitempty"`
	// Drifted says this case last held against a different model. It is not a
	// failure and not a pass: it is the reason a reader should read the rest
	// of the row differently.
	Drifted bool `json:"drifted,omitempty"`
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
	case domain.StepRunStarted:
		var p domain.RunStartedPayload
		if err := decodeInto(step, &p); err != nil {
			return err
		}
		f.c.ID = p.Case
		f.c.datasetVersion = p.DatasetVersion

	case domain.StepPlanned:
		var p domain.PlannedPayload
		if err := decodeInto(step, &p); err != nil {
			return err
		}
		f.node = p.Node
		// Kept, because a case that broke is a different fact depending on
		// whether the model under it moved.
		if p.Model != "" {
			f.c.Model, f.c.Effort = p.Model, p.Effort
		}

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
		// Both, and only one is ever set: an old run answers inline, a new one
		// by reference. Dropping either would blank one era of runs.
		f.c.Outcome, f.c.OutcomeRef = p.Outcome, p.OutcomeRef
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
