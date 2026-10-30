package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Deps are the runner's collaborators.
type Deps struct {
	Ledger  Ledger
	Gate    Gate
	Planner Planner
	// Fallback plans for a run whose ledger records a switch away from the
	// declared pair. Nil when the version declares none.
	Fallback Planner
	Tools    Tools
	Catalog  Catalog
	Clock    Clock
	Dedupe   DedupeStore
	// DedupePendingTTL is how long a reservation may sit pending before
	// another run may take it. DedupePendingWait is how long this worker waits
	// for another run's pending reservation to become confirmed before it
	// returns an operational error rather than spinning hot.
	DedupePendingTTL  time.Duration
	DedupePendingWait time.Duration
	DedupePendingPoll time.Duration
	// Content holds payloads too large or too sensitive for the ledger. A nil
	// store means references never resolve, which is fine for a run whose
	// tools return nothing bulky.
	Content ContentStore
	Metrics RuntimeMetrics
	// Standing releases parked approvals covered by a durable human grant.
	// Nil means no grants exist and every park waits for a click.
	Standing StandingApprovals
	// Schemas is the planner's own view of what a tool looks like, so the
	// runner can notice — before spending a model call — that a pack tool
	// cannot be offered at all. Nil disables the check (#148).
	Schemas Schemas
	// Servers tells a missing tool's server apart from a removed one. Nil
	// means nothing is expected, and a missing tool is only reported.
	Servers Servers
	// Connections is which servers this worker holds a live session to.
	// Parking needs it and Servers both: outage means configured, enabled
	// and not connected. Nil means connection state is unknown, and no turn
	// is refused over it.
	Connections Connections
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
	if summary, stalled := state.stalledInvestigation(); stalled {
		return r.parkInvestigation(ctx, state, start, summary)
	}
	if next, st, ok, err := r.initialMemoryLookup(ctx, state, start); ok || err != nil {
		return st, err
	} else {
		state = next
	}

	proposal, err := r.plan(ctx, state, start)
	if err != nil {
		// A turn can spend before it fails — a provider call that succeeded
		// followed by one that did not. The failure still ends the turn, but
		// the spend goes on the ledger first: released unrecorded, every
		// retry burns an invoiced call that no budget or trail ever sees.
		var spent *SpentPlanError
		if errors.As(err, &spent) {
			if r.deps.Metrics != nil {
				r.deps.Metrics.Planning(spent.Prompt, spent.Cost)
			}
			if _, aerr := r.appendPlanned(ctx, state, start, domain.Step{
				Kind:   domain.StepPlanned,
				Labels: state.Labels.Clone(),
				Cost:   spent.Cost,
				Payload: mustJSON(domain.PlannedPayload{
					Node:             StepNameAt(start, state.Called),
					Provider:         spent.Provider,
					Model:            spent.Model,
					ServedModel:      spent.ServedModel,
					Price:            pricePayload(spent.Price),
					Prompt:           promptInputPayload(spent.Prompt),
					UnavailableTools: toolStrings(spent.Unavailable),
					ClosedTools:      toolStrings(closedTools(start, state)),
				}),
			}); aerr != nil {
				return Status{}, aerr
			}
		}
		return Status{}, err
	}
	if r.deps.Metrics != nil {
		r.deps.Metrics.Planning(proposal.Prompt, proposal.Cost)
	}
	planned := domain.PlannedPayload{
		Node:        StepNameAt(start, state.Called),
		Provider:    proposal.Provider,
		Model:       proposal.Model,
		ServedModel: proposal.ServedModel,
		Price:       pricePayload(proposal.Price),
		Prompt:      promptInputPayload(proposal.Prompt),
		// What the prompt could not offer, so the trail explains a run that
		// worked without part of its pack (#148).
		UnavailableTools: toolStrings(proposal.Unavailable),
		// What forward movement put behind the run, so a narrowed envelope
		// never reads as an outage (#152).
		ClosedTools: toolStrings(closedTools(start, state)),
	}
	if state, err = r.appendPlanned(ctx, state, start, domain.Step{
		Kind:   domain.StepPlanned,
		Labels: state.Labels.Clone(),
		Cost:   proposal.Cost,
		// The step the run was in when it proposed this. Reserved on the
		// payload since the beginning and never written, because nothing knew
		// which step a run was in — that is what a correction anchors to
		// (PRD FU-13), and what lets the diagram group by stage.
		Payload: mustJSON(planned),
	}); err != nil {
		return Status{}, err
	}

	if proposal.Done {
		// The answer goes where an erasure can reach it. Same sequence
		// arithmetic as a tool's arguments: the reference points at the step
		// about to be appended, so retention works per run.
		finishSeq := state.Seq + 1
		ref, err := r.storeOutcome(ctx, start.RunID, finishSeq, proposal.Outcome)
		if err != nil {
			return Status{}, err
		}
		// The headline follows the outcome into the content store: it is
		// model-authored text that may restate what the run read, and the
		// immutable chain would put it beyond every erasure request (#165).
		headlineRef, err := r.storeOutcome(ctx, start.RunID, finishSeq, proposal.Headline)
		if err != nil {
			return Status{}, err
		}
		artifacts, err := r.storeArtifacts(ctx, start, state, finishSeq, proposal.Artifacts)
		if err != nil {
			return Status{}, err
		}
		state, err = r.append(ctx, state, start, domain.Step{
			Kind: domain.StepRunFinished,
			// What the run had learned by the time it answered. Every other
			// step that matters carries this, and the closing answer is the one
			// a memory is most often taught from — so leaving it bare meant a
			// fact remembered from an answer that restated untrusted input was
			// remembered as trustworthy.
			Labels: state.Labels.Clone(),
			Payload: mustJSON(domain.RunFinishedPayload{
				OutcomeRef:     ref,
				OutcomeDigest:  digest([]byte(proposal.Outcome)),
				Artifacts:      artifacts,
				Reason:         domain.RunFinishedByFinishTool,
				StoppedBy:      proposal.StoppedBy,
				HeadlineRef:    headlineRef,
				HeadlineDigest: headlineDigest(proposal.Headline),
			}),
		})
		return status(state), err
	}

	if proposal.Tool == "" {
		return r.park(ctx, state, start, "no_finish_action")
	}

	return r.act(ctx, state, start, proposal)
}

func (r *Runner) storeArtifacts(
	ctx context.Context, start Start, state State, seq int64, artifacts map[string]string,
) ([]domain.ContextArtifact, error) {
	if len(artifacts) == 0 {
		return nil, nil
	}
	if r.deps.Content == nil {
		return nil, fmt.Errorf("engine: no content store to hold context artifacts of %s", start.RunID)
	}

	out := make([]domain.ContextArtifact, 0, len(artifacts))
	for name, body := range artifacts {
		data := []byte(body)
		ref, err := r.deps.Content.Put(ctx, start.RunID, seq, data)
		if err != nil {
			return nil, fmt.Errorf("engine: store context artifact %q: %w", name, err)
		}
		out = append(out, domain.ContextArtifact{
			Name: name, Kind: "text", Ref: ref, Digest: digest(data),
			SourceRun: start.RunID, SourceAgent: start.AgentID,
			Labels: state.Labels.Clone(),
		})
	}
	return out, nil
}

/*
storeOutcome puts the model's closing answer in the content store.

It refuses rather than falling back to writing the text into the step. A
missing content store is a misconfigured installation, and the quiet fallback
would be the platform recording personal data in the one table an erasure
request cannot reach — the exact defect this exists to close.

An empty answer stores nothing: there is no content to erase, and a reference
to zero bytes would only be a reference that fails to resolve later.
*/
// headlineDigest is the digest of a declared headline, or nothing: hashing
// the empty string would stamp a digest for a line nobody wrote.
func headlineDigest(headline string) string {
	if headline == "" {
		return ""
	}
	return digest([]byte(headline))
}

func (r *Runner) storeOutcome(
	ctx context.Context, runID domain.RunID, seq int64, outcome string,
) (string, error) {
	if outcome == "" {
		return "", nil
	}
	if r.deps.Content == nil {
		return "", fmt.Errorf("engine: no content store to hold the outcome of %s", runID)
	}
	ref, err := r.deps.Content.Put(ctx, runID, seq, []byte(outcome))
	if err != nil {
		return "", fmt.Errorf("engine: store outcome: %w", err)
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
	planner, model, err := r.plannerFor(state, start, model)
	if err != nil {
		return Proposal{}, err
	}

	// The envelope rather than the pack, which is what this input has always
	// said it carries: a tool the step forbids should not be proposable at
	// all, and offering it spent a turn on a refusal the design promised
	// could not happen.
	at := StepAt(start, state.Called)
	offered := envelopeForState(start, state).Tools()
	unavailable, err := r.unavailableTools(ctx, offered)
	if err != nil {
		return Proposal{}, err
	}
	p, err := planner.Plan(ctx, PlanInput{
		State:          state,
		Transcript:     transcript,
		Budget:         start.Budget,
		Ceiling:        start.Ceiling,
		Remaining:      remaining(start.Budget, state.Committed()),
		Tools:          offered,
		Unavailable:    unavailable,
		MemoryLearning: start.MemoryLearning.Normalize(),
		Model:          model,
		Effort:         effort,
		Step:           StepNameAt(start, state.Called),
		StopsWhen:      stopsWhenAt(start, at),
	})
	if err != nil {
		// A billed failure still describes the prompt that was paid for,
		// and that prompt went out without the same tools.
		var spent *SpentPlanError
		if errors.As(err, &spent) {
			spent.Unavailable = unavailable
		}
		return Proposal{}, fmt.Errorf("engine: plan: %w", err)
	}
	p.Unavailable = unavailable
	return p, nil
}

/*
appendPlanned seals the turn's planning record, and when the fence refuses it,
lands the billed spend anyway.

The model call was invoiced whatever the fence says. Metrics saw it before
this write; a cost that reached Prometheus but never the ledger is exactly
the drift #131 closed, reopened on the fencing path — budget, FinOps and the
trail all fold from steps. So on ErrHeadMoved or ErrLeaseLost the cost goes
in unconditionally, idempotent under a key tied to the head it was decided
against — a pure record of spend, carrying no stale decision for anyone to
act on. The fence error still ends the turn.
*/
func (r *Runner) appendPlanned(
	ctx context.Context, state State, start Start, s domain.Step,
) (State, error) {
	// The key goes on before the FIRST write, and the fallback carries the
	// same one. A commit whose answer was swallowed — cancelled between
	// landing and returning — is indistinguishable from a lost write; only an
	// identical key lets the retry's ErrIdemConflict prove that this exact
	// charge is already on the record instead of billing it twice. Epoch in
	// the key: two attempts fenced over the same head are two real charges,
	// each under its own claim, and one claim makes one planning call — so
	// run+seq+epoch provably names one charge.
	if s.Cost != (domain.Cost{}) {
		s.IdemKey = fmt.Sprintf("planned-spend:%s:%d:%d", start.RunID, state.Seq+1, start.LeaseEpoch)
	}

	next, err := r.append(ctx, state, start, s)
	// A context already cancelled counts as fenced too: the cancellation that
	// races a billed response arrives as context.Canceled on the append, and
	// this turn will never retry the write.
	fenced := errors.Is(err, domain.ErrHeadMoved) || errors.Is(err, domain.ErrLeaseLost) ||
		(err != nil && ctx.Err() != nil)
	if !fenced || s.Cost == (domain.Cost{}) {
		return next, err
	}

	// The spend write gets a context of its own: the cancellation that fenced
	// the turn must not also strand the invoice.
	spendCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stop()

	spend := r.stamp(start, s)
	if _, aerr := r.deps.Ledger.Append(spendCtx, spend); aerr != nil && !errors.Is(aerr, domain.ErrIdemConflict) {
		return State{}, fmt.Errorf("engine: billed spend not recorded after the fence (%v): %w", err, aerr)
	}
	return State{}, err
}

// stamp fills the identity every step of this run carries.
func (r *Runner) stamp(start Start, s domain.Step) domain.Step {
	s.RunID = start.RunID
	s.Scope = start.Scope
	s.AgentID = start.AgentID
	s.VersionID = start.VersionID
	s.OnBehalfOf = start.OnBehalfOf
	s.At = r.deps.Clock.Now()
	return s
}

// append seals one step into the ledger and folds it into the state, keeping
// the in-memory view and the record identical by construction.
func (r *Runner) append(ctx context.Context, state State, start Start, s domain.Step) (State, error) {
	s = r.stamp(start, s)

	// Conditional on the head this turn folded: a run that moved in between —
	// a successor worker, an approval, an abandonment — refuses the write with
	// ErrHeadMoved rather than sealing a stale decision onto it (NF-15). Under
	// a lease, conditional on the epoch too: a successor's claim moves no
	// steps, so the head alone cannot tell a fenced worker it lost the run.
	// Only the opening step has no head to condition on.
	append := func() (domain.Step, error) {
		head := state.Head
		if state.Seq < domain.FirstSeq {
			head = domain.StepRef{}
		}
		if start.LeaseEpoch != 0 {
			return r.deps.Ledger.AppendIfLeaseAndHead(ctx, start.LeaseEpoch, head, s)
		}
		if head == (domain.StepRef{}) {
			return r.deps.Ledger.Append(ctx, s)
		}
		return r.deps.Ledger.AppendIfHead(ctx, head, s)
	}
	sealed, err := append()
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

func promptInputPayload(p domain.PromptInputBreakdown) *domain.PromptInputBreakdown {
	if p.Unit == "" && p.Total == 0 {
		return nil
	}
	return &p
}

func pricePayload(p domain.ModelPriceUse) *domain.ModelPriceUse {
	if p.Status == "" {
		return nil
	}
	return &p
}

// isNotFound asks the error what it is rather than what it says.
//
// It compared the message text, which made the sentence a contract nobody could
// see: rewording it — or pointing the ledger's sentinel at the shared one, as
// telling a purged run from an unreachable database required — silently turned
// "this run has not started" into a fatal read error, and a first Advance could
// no longer open a run.
func isNotFound(err error) bool {
	return errors.Is(err, domain.ErrRunNotFound)
}
