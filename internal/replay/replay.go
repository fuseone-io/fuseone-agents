/*
Package replay answers what a policy would have changed (PRD AU-08).

It re-evaluates past decisions rather than replaying them. The difference is
the whole requirement: a record of what was decided can be shown again, and
only a record of what it was decided *from* can be decided differently.

The one way this can be wrong in a way that matters is by answering "nothing
would change" about a decision it could not actually re-evaluate. Somebody is
about to publish a policy on the strength of the answer, so a decision whose
inputs are not in the ledger is reported as unanswerable and never as
unchanged.
*/
package replay

import (
	"encoding/json"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

// Difference is one decision the new set would have ruled differently.
type Difference struct {
	RunID domain.RunID   `json:"runId"`
	Seq   int64          `json:"seq"`
	Tool  domain.ToolID  `json:"tool"`
	Was   domain.Verdict `json:"was"`
	Now   domain.Verdict `json:"now"`
	// Policy is the authored rule that would decide it now, when one does.
	Policy string `json:"policy,omitempty"`
}

// Unanswerable is a decision the ledger cannot answer for.
type Unanswerable struct {
	RunID domain.RunID  `json:"runId"`
	Seq   int64         `json:"seq"`
	Tool  domain.ToolID `json:"tool"`
	// Policy is the rule that cannot be checked against it.
	Policy string `json:"policy"`
	Reason string `json:"reason"`
}

// Report is what a proposed policy set would have done to a range of history.
type Report struct {
	Evaluated   int            `json:"evaluated"`
	Changed     []Difference   `json:"changed,omitempty"`
	Unevaluable []Unanswerable `json:"unevaluable,omitempty"`
}

// Against re-evaluates every decision in a range against a policy set.
func Against(steps []domain.Step, policies []domain.Policy) Report {
	var report Report

	for _, step := range steps {
		if step.Kind != domain.StepGateDecided {
			continue
		}
		var recorded domain.GateDecidedPayload
		if err := json.Unmarshal(step.Payload, &recorded); err != nil {
			continue
		}

		in := domain.PolicyInput{
			Tool: recorded.Tool, Effect: recorded.Effect,
			Agent: step.AgentID, Scope: step.Scope, Labels: recorded.Labels,
			// Args are deliberately absent: they are not in the ledger, and a
			// zero value here would silently make every args rule read as
			// not matching.
		}

		if blocked := unanswerable(step, recorded, in, policies); blocked != nil {
			report.Unevaluable = append(report.Unevaluable, *blocked)
			continue
		}

		report.Evaluated++
		now, by := verdictOf(in, policies)
		if now == recorded.Verdict {
			continue
		}
		report.Changed = append(report.Changed, Difference{
			RunID: step.RunID, Seq: step.Seq, Tool: recorded.Tool,
			Was: recorded.Verdict, Now: now, Policy: by,
		})
	}
	return report
}

/*
unanswerable reports the rule that stops this decision being re-evaluated.

Only a rule that could actually have applied. A policy reading argument
content but scoped to another tool could never have touched this call, and
refusing to answer because of it would make the report useless the moment an
installation has one such rule anywhere.
*/
func unanswerable(
	step domain.Step, recorded domain.GateDecidedPayload,
	in domain.PolicyInput, policies []domain.Policy,
) *Unanswerable {
	for _, policy := range policies {
		if !policy.Enabled || !policy.Applies(in) {
			continue
		}
		for _, condition := range policy.Conditions {
			if !strings.HasPrefix(condition.Field, "args.") {
				continue
			}
			return &Unanswerable{
				RunID: step.RunID, Seq: step.Seq, Tool: recorded.Tool,
				Policy: policy.Code,
				// Stated as the fact it is. The arguments are not kept
				// because they carry whatever the case carried, and this is
				// what that costs.
				Reason: "the rule reads argument content, which the ledger does not keep",
			}
		}
	}
	return nil
}

// verdictOf is the most restrictive ruling the set produces, and which rule
// produced it. Monitored policies are excluded: they change nothing today and
// would change nothing after this is published either.
func verdictOf(in domain.PolicyInput, policies []domain.Policy) (domain.Verdict, string) {
	worst, by := domain.VerdictAllow, ""
	for _, policy := range policies {
		if !policy.Enabled || policy.Mode == domain.PolicyMonitor || !policy.Matches(in) {
			continue
		}
		if verdict := policy.Effect.Verdict(); verdict > worst {
			worst, by = verdict, policy.Code
		}
	}
	return worst, by
}
