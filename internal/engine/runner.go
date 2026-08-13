package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
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

// act runs the proposal through the Gate and, if it survives, executes it.
func (r *Runner) act(ctx context.Context, state State, start Start, p Proposal) (Status, error) {
	effect, _ := r.deps.Catalog.Effect(p.Tool)
	idemKey := idempotencyKey(start.RunID, p.Tool, p.Args)

	decision, err := r.deps.Gate.Evaluate(ctx, gate.Request{
		Scope:           start.Scope,
		RunID:           start.RunID,
		AgentID:         start.AgentID,
		Seq:             state.Seq + 1,
		Tool:            p.Tool,
		Effect:          effect,
		Args:            p.Args,
		ArgLabels:       state.Labels,
		Pack:            envelopeOf(start, state.Called),
		Stage:           start.Stage,
		Budget:          start.Budget,
		Committed:       state.Committed(),
		Estimate:        p.Estimate,
		IdemKey:         idemKey,
		AlreadyExecuted: state.AlreadyExecuted(idemKey),
	})
	if err != nil {
		return Status{}, fmt.Errorf("engine: gate: %w", err)
	}

	state, err = r.append(ctx, state, start, domain.Step{
		Kind:       domain.StepGateDecided,
		PolicyHash: decision.PolicyHash,
		Payload: mustJSON(domain.GateDecidedPayload{
			Tool: p.Tool, Effect: effect, Verdict: decision.Verdict,
			Rule: decision.Rule, Reason: decision.Reason,
			PolicyCode: decision.PolicyCode, Monitored: decision.Monitored,
			// The inputs beside the outcome, so this decision can be
			// re-evaluated later and not merely replayed (AU-08).
			Labels: state.Labels, ArgsDigest: digest(p.Args),
		}),
	})
	if err != nil {
		return Status{}, err
	}

	switch {
	case decision.Verdict == domain.VerdictRequireApproval:
		argsRef, err := r.store(ctx, start.RunID, state.Seq+1, p.Args)
		if err != nil {
			return Status{}, err
		}
		state, err = r.append(ctx, state, start, domain.Step{
			Kind: domain.StepApprovalRequested,
			Payload: mustJSON(domain.ApprovalRequestedPayload{
				Tool: p.Tool, Rule: decision.Rule, Reason: decision.Reason,
				Effect: effect, ArgsRef: argsRef, ArgsDigest: digest(p.Args),
				Estimate: p.Estimate, Labels: state.Labels,
			}),
		})
		return status(state), err

	case decision.Rule == gate.RuleBudget:
		// A budget block parks the run rather than failing it: raising the
		// ceiling resumes from this exact step (PRD FO-04).
		state, err = r.append(ctx, state, start, domain.Step{
			Kind:    domain.StepParked,
			Payload: mustJSON(domain.ParkedPayload{Reason: "budget_exhausted"}),
		})
		return status(state), err

	case !decision.Verdict.Executable() && state.ConsecutiveBlocks >= maxConsecutiveBlocks:
		// The refusal was fed back and the planner kept asking anyway. Waiting
		// for the step ceiling to end this does not work: the Gate reports the
		// most restrictive rule, so a capability refusal masks the budget one,
		// and every further turn is a paid model call that cannot succeed.
		state, err = r.append(ctx, state, start, domain.Step{
			Kind:    domain.StepParked,
			Payload: mustJSON(domain.ParkedPayload{Reason: "no_progress"}),
		})
		return status(state), err

	case !decision.Verdict.Executable():
		// A first refusal is recorded and fed back so the planner can choose
		// differently. Most refusals cannot be argued with — the pack, the
		// taint and the policy are fixed for the run's version — but a
		// contract refusal is one the model can genuinely fix (PRD SE-09).
		return status(state), nil
	}

	return r.invoke(ctx, state, start, p, effect, idemKey)
}

// invoke reserves budget, calls the tool and reconciles.
func (r *Runner) invoke(
	ctx context.Context, state State, start Start,
	p Proposal, effect domain.Effect, idemKey string,
) (Status, error) {
	state, err := r.append(ctx, state, start, domain.Step{
		Kind: domain.StepBudgetReserved,
		Payload: mustJSON(domain.BudgetReservedPayload{
			Micros: p.Estimate.Micros, Tokens: p.Estimate.Tokens,
		}),
	})
	if err != nil {
		return Status{}, err
	}

	argsRef, err := r.store(ctx, start.RunID, state.Seq+1, p.Args)
	if err != nil {
		return Status{}, err
	}

	// The idempotency key is recorded with the call, before the effect leaves
	// the process. A crash after this append means the resume sees the key and
	// refuses to call again (PRD DE-16).
	if state, err = r.append(ctx, state, start, domain.Step{
		Kind:    domain.StepToolCalled,
		IdemKey: idemKey,
		Payload: mustJSON(domain.ToolCalledPayload{
			Tool: p.Tool, Effect: effect, ArgsRef: argsRef, ArgsDigest: digest(p.Args),
		}),
	}); err != nil {
		return Status{}, err
	}

	result, invokeErr := r.deps.Tools.Invoke(ctx, Call{
		RunID: start.RunID, Seq: state.Seq,
		Tool: p.Tool, Args: p.Args, IdemKey: idemKey,
	})
	returned := domain.ToolReturnedPayload{Tool: p.Tool, ResultRef: result.ResultRef}
	if invokeErr != nil {
		returned.Failed = true
		returned.ErrorCode = "invoke_error"
	}

	if state, err = r.append(ctx, state, start, domain.Step{
		Kind:    domain.StepToolReturned,
		Cost:    result.Cost,
		Labels:  result.Labels,
		Payload: mustJSON(returned),
	}); err != nil {
		return Status{}, err
	}

	// Reconcile on every path, including failure. A reservation left
	// outstanding leaks budget for the rest of the run and eventually parks it
	// for no reason.
	state, err = r.append(ctx, state, start, domain.Step{
		Kind: domain.StepBudgetReconciled,
		Payload: mustJSON(domain.BudgetReconciledPayload{
			ReleasedMicros: p.Estimate.Micros, ReleasedTokens: p.Estimate.Tokens,
		}),
	})
	return status(state), err
}

// store puts the proposed arguments in the content store and returns the
// reference the ledger records instead of the bytes.
//
// The sequence is the one the step about to be appended will take, so a
// reference points at the step it belongs to and retention can work per run.
func (r *Runner) store(ctx context.Context, runID domain.RunID, seq int64, args []byte) (string, error) {
	if len(args) == 0 || r.deps.Content == nil {
		return "", nil
	}
	ref, err := r.deps.Content.Put(ctx, runID, seq, args)
	if err != nil {
		return "", fmt.Errorf("engine: store arguments: %w", err)
	}
	return ref, nil
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

// idempotencyKey identifies an effect by what it does, not by where it sits.
//
// The step sequence is deliberately excluded. A resumed run re-plans and lands
// at a different sequence number, so a position-dependent key would look new
// on every retry and duplicate the effect — the exact failure this key exists
// to prevent.
//
// The consequence is that an identical call with identical arguments inside
// one run happens once. For reads that is a free cache hit; for writes a
// second identical call is almost always a bug. A tool that genuinely needs to
// repeat — polling, pagination — varies its arguments, and that is the
// intended escape hatch.
func idempotencyKey(runID domain.RunID, tool domain.ToolID, args []byte) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|", runID, tool)
	_, _ = h.Write(args)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
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
