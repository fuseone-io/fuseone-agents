package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// Deps are the runner's collaborators.
type Deps struct {
	Ledger  Ledger
	Gate    Gate
	Planner Planner
	Tools   Tools
	Catalog Catalog
	Clock   Clock
	// Content holds payloads too large or too sensitive for the ledger. A nil
	// store means references never resolve, which is fine for a run whose
	// tools return nothing bulky.
	Content ContentStore
}

// maxConsecutiveBlocks bounds how long the platform argues with a planner that
// will not take no for an answer. Low on purpose: every turn is a paid model
// call, and a run that has been refused this many times in a row needs a
// person, not another attempt.
const maxConsecutiveBlocks = 3

// Runner executes the fixed agent loop: plan, gate, execute, append.
//
// It owns no state of its own. Everything it needs comes from folding the
// ledger, which is what lets any worker pick up any run at any time.
type Runner struct {
	deps Deps
}

func NewRunner(d Deps) *Runner { return &Runner{deps: d} }

// Advance performs one turn of the loop and returns where the run stands.
//
// One turn is at most one planning call and at most one tool call. Keeping the
// unit small is what makes a crash cheap: the ledger is consistent after every
// append, so a resume never has more than one turn to reconstruct.
func (r *Runner) Advance(ctx context.Context, start Start) (Status, error) {
	state, err := r.load(ctx, start)
	if err != nil {
		return Status{}, err
	}
	if state.Terminal() || state.Phase == PhaseAwaitingApproval {
		return status(state), nil
	}
	if state.Phase == PhaseAwaitingTool {
		return r.recoverOrphanedCall(ctx, state, start)
	}
	// A granted approval is an instruction, not a suggestion to reconsider.
	// Planning here would ask the model what to do next and execute whatever
	// it said — on the strength of a person agreeing to something else.
	if state.Approved != nil {
		return r.actApproved(ctx, state, start)
	}

	proposal, err := r.plan(ctx, state, start)
	if err != nil {
		return Status{}, err
	}
	if state, err = r.append(ctx, state, start, domain.Step{
		Kind: domain.StepPlanned,
		Cost: proposal.Cost,
		// The step the run was in when it proposed this. Reserved on the
		// payload since the beginning and never written, because nothing knew
		// which step a run was in — that is what a correction anchors to
		// (PRD FU-13), and what lets the diagram group by stage.
		Payload: mustJSON(domain.PlannedPayload{Node: StepNameAt(start, state.Called)}),
	}); err != nil {
		return Status{}, err
	}

	if proposal.Done {
		state, err = r.append(ctx, state, start, domain.Step{
			Kind:    domain.StepRunFinished,
			Payload: mustJSON(domain.RunFinishedPayload{Outcome: proposal.Outcome}),
		})
		return status(state), err
	}

	return r.act(ctx, state, start, proposal)
}

func (r *Runner) plan(ctx context.Context, state State, start Start) (Proposal, error) {
	// The transcript is rebuilt from the ledger every turn rather than carried
	// in memory. That is what lets any worker pick up any run: the model's
	// view of the conversation is derived, never held.
	steps, err := r.deps.Ledger.Read(ctx, start.RunID, domain.FirstSeq)
	if err != nil {
		return Proposal{}, fmt.Errorf("engine: read for transcript: %w", err)
	}
	transcript, err := BuildTranscript(ctx, r.deps.Content, steps)
	if err != nil {
		return Proposal{}, err
	}

	// What this step is worth spending on. A step that classifies a ticket and
	// one that decides what to do about it are the same run and not the same
	// expense (PRD FO-10, FO-11).
	model, effort := SpendAt(start, state.Called)

	p, err := r.deps.Planner.Plan(ctx, PlanInput{
		State:      state,
		Transcript: transcript,
		Budget:     start.Budget,
		Remaining:  remaining(start.Budget, state.Committed()),
		Tools:      start.Pack.Tools(),
		Model:      model,
		Effort:     effort,
	})
	if err != nil {
		return Proposal{}, fmt.Errorf("engine: plan: %w", err)
	}
	return p, nil
}

// append seals one step into the ledger and folds it into the state, keeping
// the in-memory view and the record identical by construction.
func (r *Runner) append(ctx context.Context, state State, start Start, s domain.Step) (State, error) {
	s.RunID = start.RunID
	s.Scope = start.Scope
	s.AgentID = start.AgentID
	s.VersionID = start.VersionID
	s.OnBehalfOf = start.OnBehalfOf
	s.At = r.deps.Clock.Now()

	sealed, err := r.deps.Ledger.Append(ctx, s)
	if err != nil {
		return State{}, fmt.Errorf("engine: append %s: %w", s.Kind, err)
	}
	if err := state.Apply(sealed); err != nil {
		return State{}, err
	}
	return state, nil
}

func status(s State) Status {
	return Status{Phase: s.Phase, Seq: s.Seq, Done: s.Terminal()}
}

func remaining(b domain.Budget, c domain.Consumption) domain.Consumption {
	return domain.Consumption{
		Micros:    max(0, b.Micros-c.Micros),
		Tokens:    max(0, b.Tokens-c.Tokens),
		ToolCalls: max(0, b.ToolCalls-c.ToolCalls),
		Steps:     max(0, b.Steps-c.Steps),
	}
}

func mustJSON(v any) []byte {
	// The payload types are closed structs of scalars and strings; marshalling
	// one cannot fail, and a nil payload would corrupt the audit record.
	b, err := json.Marshal(v)
	if err != nil {
		panic("engine: payload is not serialisable: " + err.Error())
	}
	return b
}

func isNotFound(err error) bool {
	return err != nil && err.Error() == "ledger: run not found"
}
