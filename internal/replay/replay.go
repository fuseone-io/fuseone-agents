/*
Package replay reconstructs a run exactly as it happened (PRD AU-07).

The hash chain proves the steps were not edited. It does not prove they were
ever the correct output of the rules the platform says were in force — a chain
of well-sealed lies verifies perfectly. Faithful replay is the other half:
feed the recorded inputs back through the Gate, under the policy set the step
itself names, and check that today's answer is the answer that was recorded.

A mismatch means one of three things, and the report says which is possible
rather than guessing: the trail was altered before it was sealed, the policy
snapshot under that hash is not what produced it, or the Gate's own behaviour
has changed since. All three are worth an afternoon; none of them is visible
without doing this.

It changes nothing and needs no tool, no model and no network. An auditor can
run it against a cold copy of the database.
*/
package replay

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

// Snapshots resolve the policy set a decision was made under, declared here by
// the consumer.
type Snapshots interface {
	Snapshot(ctx context.Context, hash string) ([]domain.Policy, error)
}

// Specs resolve the capability pack of the exact version that ran.
//
// The version, not the current one: publishing after a run must not change
// what the run is replayed against, which is the same reason the run pinned a
// version in the first place (PRD DE-09).
type Specs interface {
	Pack(ctx context.Context, agent domain.AgentID, version domain.VersionID) (gate.Pack, error)
}

// Divergence is one decision that would not be made the same way again.
type Divergence struct {
	Seq  int64
	Tool domain.ToolID
	// Was is what the trail records; Now is what the same inputs produce
	// today, under the same policies.
	Was domain.Verdict
	Now domain.Verdict
	// WasRule and NowRule matter as much as the verdict: two rules that reach
	// the same answer for different reasons is still a change in behaviour.
	WasRule string
	NowRule string
	// Why is set when the decision could not be re-derived at all — a policy
	// snapshot nobody kept, most often. Not a divergence but not a match
	// either, and reporting it as either would be a lie.
	Why string
}

// Report is what a replay found.
type Report struct {
	RunID domain.RunID
	// Decisions is how many gate decisions the run recorded.
	Decisions int
	// Reproduced is how many produced the same verdict and rule again.
	Reproduced  int
	Divergences []Divergence
}

// Faithful reports whether every decision came out the same way.
func (r Report) Faithful() bool {
	return r.Decisions > 0 && r.Reproduced == r.Decisions
}

/*
Run replays a run's decisions against the policies in force at the time.

The Gate is constructed per decision, from that step's own policy hash: a run
that spanned a policy change recorded two hashes, and replaying the whole run
under either one would report the other half as diverged.
*/
func Run(ctx context.Context, steps []domain.Step, snapshots Snapshots, specs Specs) (Report, error) {
	if len(steps) == 0 {
		return Report{}, fmt.Errorf("replay: nothing to replay")
	}
	report := Report{RunID: steps[0].RunID}

	// The pack of the version that ran, not of the version published now.
	pack, err := specs.Pack(ctx, steps[0].AgentID, steps[0].VersionID)
	if err != nil {
		return Report{}, fmt.Errorf("replay: pack of %s@%s: %w",
			steps[0].AgentID, steps[0].VersionID, err)
	}

	sets := map[string][]domain.Policy{}
	for _, step := range steps {
		if step.Kind != domain.StepGateDecided {
			continue
		}
		report.Decisions++

		var recorded domain.GateDecidedPayload
		if err := decode(step, &recorded); err != nil {
			report.Divergences = append(report.Divergences, Divergence{
				Seq: step.Seq, Why: "the recorded decision cannot be read",
			})
			continue
		}

		policies, err := setFor(ctx, sets, snapshots, step.PolicyHash)
		if err != nil {
			report.Divergences = append(report.Divergences, Divergence{
				Seq: step.Seq, Tool: recorded.Tool, Was: recorded.Verdict,
				WasRule: recorded.Rule, Why: err.Error(),
			})
			continue
		}

		// The trust in force then. Decisions recorded before the platform
		// wrote it down cannot be re-derived: the autonomy check would run
		// under an unset stage and refuse everything, and reporting that as a
		// divergence would be blaming the trail for a gap in the record.
		if !recorded.Stage.Valid() {
			report.Divergences = append(report.Divergences, Divergence{
				Seq: step.Seq, Tool: recorded.Tool, Was: recorded.Verdict,
				WasRule: recorded.Rule,
				Why:     "the decision does not record the trust stage",
			})
			continue
		}

		again, err := replayOne(ctx, pack, step, recorded, policies)
		if err != nil {
			return report, err
		}
		if again.Verdict == recorded.Verdict && again.Rule == recorded.Rule {
			report.Reproduced++
			continue
		}
		report.Divergences = append(report.Divergences, Divergence{
			Seq: step.Seq, Tool: recorded.Tool,
			Was: recorded.Verdict, Now: again.Verdict,
			WasRule: recorded.Rule, NowRule: again.Rule,
		})
	}
	return report, nil
}

// replayOne asks the Gate the same question the run asked it.
//
// The arguments are the one input not replayed: they are behind a reference
// that erasure may legitimately have emptied, and a replay that failed because
// somebody exercised their rights would be useless exactly when it is needed.
// The digest is on the step, so what the arguments *were* is still provable;
// what is re-derived is every check that does not read them.
func replayOne(
	ctx context.Context, pack gate.Pack, step domain.Step,
	recorded domain.GateDecidedPayload, policies []domain.Policy,
) (domain.Decision, error) {
	g := gate.New().WithPolicies(gate.Policies{
		Hash: step.PolicyHash, Set: policies,
	})

	decision, err := g.Evaluate(ctx, gate.Request{
		Scope: step.Scope, RunID: step.RunID, AgentID: step.AgentID,
		Seq: step.Seq, Tool: recorded.Tool, Effect: recorded.Effect,
		ArgLabels: recorded.Labels,
		Pack:      pack, Stage: recorded.Stage,
		// Reproducing the outcome, not the ceiling: budget is a fact about
		// when the run happened, and a replay months later would report every
		// decision as diverged for want of headroom that no longer exists.
		Budget: domain.Budget{},
		// An approval that was granted then was granted; replaying it as
		// ungranted would report the whole second half of the run as changed.
		ApprovalGranted: recorded.Verdict.Executable() &&
			recorded.Rule == gate.RuleApproval,
	})
	if err != nil {
		return domain.Decision{}, fmt.Errorf("replay: gate at %d: %w", step.Seq, err)
	}
	return decision, nil
}

// setFor resolves a policy hash once per replay rather than once per decision.
func setFor(
	ctx context.Context, cache map[string][]domain.Policy,
	snapshots Snapshots, hash string,
) ([]domain.Policy, error) {
	if hash == "" {
		return nil, nil
	}
	if set, ok := cache[hash]; ok {
		return set, nil
	}
	set, err := snapshots.Snapshot(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("the policy set %s was not kept", hash)
	}
	cache[hash] = set
	return set, nil
}

// decode reads a step's payload. Local rather than shared: this package does
// not import the engine, and a replay that could only run where the loop runs
// would not be the independent check it is meant to be.
func decode(step domain.Step, into any) error {
	if len(step.Payload) == 0 {
		return fmt.Errorf("replay: step %d carries no payload", step.Seq)
	}
	return json.Unmarshal(step.Payload, into)
}
