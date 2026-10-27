package compensate

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
)

/*
Performing a compensation.

Never automatically. What ends a run badly in this platform is parking, and
parking is a pause: a run stopped at its ceiling resumes when somebody raises
it, and compensating one would undo work it was about to continue. So this runs
when a person decides the run is over and cannot go on — which is also the only
sane trigger for the most dangerous code here.

Every compensating call crosses the Gate. There is no "just this once" on the
failure path either, and the permission it stands on is the narrow one: you may
undo what you were allowed to do.
*/

// Deps are what performing needs. Declared here by the consumer, and the same
// collaborators the loop uses — a compensation that reached tools by another
// route would be an effect outside everything that governs effects.
type Deps struct {
	Ledger  engine.Ledger
	Gate    engine.Gate
	Tools   engine.Tools
	Catalog engine.Catalog
	Clock   engine.Clock
	// Content resolves what the original call returned, which is what the undo
	// is called with. Nil means the undo goes out with no arguments, which
	// suits a tool that needs none and fails loudly for one that does — the
	// failure being recorded, not swallowed.
	Content engine.ContentStore
}

// Outcome is what became of one act.
type Outcome struct {
	Act Act
	// Done is true when the undo ran and the tool did not report a failure.
	Done bool
	// Why is empty on success, and on failure says what stopped it. A
	// compensation that could not run is the thing somebody has to act on.
	Why string
}

/*
Perform undoes what it can and records every attempt.

It does not stop at the first failure. The acts are independent — a refund that
cannot be issued is no reason to leave an order standing as well — and stopping
would leave more behind than continuing, which is the opposite of the point.

Each attempt is written to the same ledger before the external call. A second
run therefore does not repeat an undo whose result is absent: for a generic
tool, absence means unknown rather than safe to retry.
*/
func Perform(
	ctx context.Context, deps Deps, start engine.Start, plan []Act,
) ([]Outcome, error) {
	out := make([]Outcome, 0, len(plan))

	for _, act := range plan {
		if act.Undo == "" {
			// Nothing takes it back. Recorded as standing rather than skipped:
			// this is the half of the answer somebody most needs.
			out = append(out, Outcome{Act: act, Why: "nothing undoes this tool"})
			continue
		}
		if act.Failed {
			out = append(out, Outcome{Act: act, Why: "the recorded compensation failed"})
			continue
		}
		if act.Attempted {
			out = append(out, Outcome{
				Act: act, Why: "compensation outcome unknown; manual reconciliation required",
			})
			continue
		}
		outcome, err := performOne(ctx, deps, start, act)
		if err != nil {
			return out, err
		}
		out = append(out, outcome)
	}
	return out, nil
}

func performOne(
	ctx context.Context, deps Deps, start engine.Start, act Act,
) (Outcome, error) {
	effect, _ := deps.Catalog.Effect(act.Undo)

	head, err := deps.Ledger.Head(ctx, start.RunID)
	if err != nil {
		return Outcome{}, fmt.Errorf("compensate: read %s: %w", start.RunID, err)
	}
	seq := head.Seq + 1

	decision, err := deps.Gate.Evaluate(ctx, gate.Request{
		Scope: start.Scope, RunID: start.RunID, AgentID: start.AgentID, Seq: seq,
		Tool: act.Undo, Effect: effect,
		// The permission it borrows, and the only reason it may cross.
		Compensating: act.Tool,
		Pack:         start.Pack, Stage: start.Stage,
		Budget: start.Budget,
		// A compensation that a person asked for is not a proposal a copilot
		// made: they have already decided. Asking them to approve their own
		// instruction is a dialogue nobody reads.
		ApprovalGranted: true,
		IdemKey:         fmt.Sprintf("compensate:%s:%d", start.RunID, act.Seq),
	})
	if err != nil {
		return Outcome{}, fmt.Errorf("compensate: gate: %w", err)
	}
	if !decision.Verdict.Executable() {
		return recordBeforeInvoke(ctx, deps, start, act,
			fmt.Sprintf("refused by %s", decision.Rule))
	}

	args, err := undoArgs(ctx, deps, act)
	if err != nil {
		return recordBeforeInvoke(ctx, deps, start, act, err.Error())
	}

	idemKey := fmt.Sprintf("compensate:%s:%d", start.RunID, act.Seq)
	call := engine.Call{
		RunID: start.RunID, Scope: start.Scope,
		AgentID: start.AgentID, Tool: act.Undo, Args: args,
		OnBehalfOf: start.OnBehalfOf,
		IdemKey:    idemKey,
	}
	if err := deps.Tools.Reserve(ctx, call); err != nil {
		return recordBeforeInvoke(ctx, deps, start, act, err.Error())
	}
	started, err := recordStarted(ctx, deps, start, act, head, idemKey)
	if err != nil {
		return Outcome{}, err
	}
	call.Seq = started.Seq
	// The last answer before the undo leaves the process: these calls act on
	// the world, and a worker that lost its lease mid-compensation would run
	// them beside the successor doing the same. The durable intent is now the
	// exact head: a moved head means this act no longer owns the next effect.
	if start.LeaseEpoch != 0 {
		startedHead := domain.StepRef{Seq: started.Seq, Kind: started.Kind}
		if err := deps.Ledger.Fence(ctx, start.RunID, start.LeaseEpoch, startedHead); err != nil {
			return Outcome{}, fmt.Errorf("compensate: fence before %s: %w", act.Undo, err)
		}
	}
	result, invokeErr := deps.Tools.Invoke(ctx, call)
	// Both shapes of failure. A tool layer reports a refusal from the far side
	// in the result and a broken connection as an error, and an undo that did
	// not happen is the same fact either way.
	if invokeErr != nil {
		return record(ctx, deps, start, act, false, invokeErr.Error())
	}
	if result.Failed {
		return record(ctx, deps, start, act, false, codeOr(result.ErrorCode))
	}
	return record(ctx, deps, start, act, true, "")
}

// recordStarted is the write-ahead intent for an external undo. Once this is
// sealed, absence of a result means unknown rather than safe-to-retry.
func recordStarted(
	ctx context.Context, deps Deps, start engine.Start, act Act,
	head domain.Step, idemKey string,
) (domain.Step, error) {
	payload, err := jsonOf(domain.CompensationStartedPayload{
		Tool: act.Undo, ForSeq: act.Seq,
	})
	if err != nil {
		return domain.Step{}, err
	}
	s := domain.Step{
		RunID: start.RunID, Kind: domain.StepCompensationStarted,
		Scope: start.Scope, AgentID: start.AgentID, VersionID: start.VersionID,
		OnBehalfOf: start.OnBehalfOf, At: deps.Clock.Now(), Payload: payload,
		IdemKey: idemKey,
	}
	var sealed domain.Step
	expected := domain.StepRef{Seq: head.Seq, Kind: head.Kind}
	if start.LeaseEpoch != 0 {
		sealed, err = deps.Ledger.AppendIfLeaseAndHead(
			ctx, start.LeaseEpoch, expected, s,
		)
	} else {
		sealed, err = deps.Ledger.AppendIfHead(ctx, expected, s)
	}
	if err != nil {
		return domain.Step{}, fmt.Errorf("compensate: record start of %s: %w", act.Undo, err)
	}
	return sealed, nil
}

/*
undoArgs is what the original call answered with.

The undo takes what the do returned. A tool that creates something answers with
its identifier, and the tool that removes it asks for that identifier — the
shape almost every reversible API already has. The alternative is a mapping
between two schemas, declared by somebody who wrote neither tool and audited by
nobody, on the one path where being wrong means acting twice on the world.
*/
func undoArgs(ctx context.Context, deps Deps, act Act) ([]byte, error) {
	if act.ResultRef == "" || deps.Content == nil {
		return nil, nil
	}
	args, err := deps.Content.Get(ctx, act.ResultRef)
	if err != nil {
		return nil, fmt.Errorf("the original result could not be read: %w", err)
	}
	return args, nil
}

func codeOr(code string) string {
	if code == "" {
		return "the tool reported a failure"
	}
	return code
}

// record appends the result of an attempt, succeeded or not.
//
// Both are written. A compensation that failed is the most important line in
// the trail: it is the one that says something is still standing and nobody
// has dealt with it.
func record(
	ctx context.Context, deps Deps, start engine.Start,
	act Act, succeeded bool, why string,
) (Outcome, error) {
	return recordResult(ctx, deps, start, act, succeeded, false, why)
}

// recordBeforeInvoke records a failure while it is still provable that no
// external effect left this process. Recovery may safely evaluate it again.
func recordBeforeInvoke(
	ctx context.Context, deps Deps, start engine.Start, act Act, why string,
) (Outcome, error) {
	return recordResult(ctx, deps, start, act, false, true, why)
}

func recordResult(
	ctx context.Context, deps Deps, start engine.Start,
	act Act, succeeded, beforeInvoke bool, why string,
) (Outcome, error) {
	payload, err := jsonOf(domain.CompensatedPayload{
		Tool: act.Undo, ForSeq: act.Seq, Succeeded: succeeded,
		BeforeInvoke: beforeInvoke,
	})
	if err != nil {
		return Outcome{}, err
	}

	s := domain.Step{
		RunID: start.RunID, Kind: domain.StepCompensated,
		Scope: start.Scope, AgentID: start.AgentID, VersionID: start.VersionID,
		OnBehalfOf: start.OnBehalfOf, At: deps.Clock.Now(), Payload: payload,
	}
	// Under a lease, the record is fenced on it — lease only, because this
	// loop's own records move the head between acts.
	var appendErr error
	if start.LeaseEpoch != 0 {
		_, appendErr = deps.Ledger.AppendIfLeaseAndHead(ctx, start.LeaseEpoch, domain.StepRef{}, s)
	} else {
		_, appendErr = deps.Ledger.Append(ctx, s)
	}
	if appendErr != nil {
		return Outcome{}, fmt.Errorf("compensate: record %s: %w", act.Undo, appendErr)
	}
	return Outcome{Act: act, Done: succeeded, Why: why}, nil
}
