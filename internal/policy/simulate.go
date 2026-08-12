package policy

import (
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

// Simulating a draft rule against what already happened.
//
// The handoff's screen runs a rule against recent runs before saving, so a
// policy is never saved blind. The same machinery answers the PRD's harder
// question — re-evaluating a past decision under a policy written since — and
// both share one honesty problem.
//
// A rule that reads `args.*` cannot be replayed against every decision. A
// blocked call never stored arguments to store, so for those the answer is not
// "no match", it is "unknown". Reporting them as no-match would produce a
// simulation showing zero denials for exactly the rule somebody was nervous
// about, which is worse than showing nothing.

// Simulation is what a draft rule would have done.
type Simulation struct {
	// Considered is how many recorded decisions were examined.
	Considered int
	// Matched is how many the rule would have applied to.
	Matched int
	// Unknown is how many could not be answered, because the rule reads
	// arguments and those decisions did not keep any.
	Unknown int
	// ByVerdict counts what the rule would have produced, among the matches.
	ByVerdict map[domain.Verdict]int
	// Samples are a few of the matches, so a number has runs behind it.
	Samples []Sample
}

// Sample is one decision the rule would have applied to.
type Sample struct {
	RunID   domain.RunID
	Seq     int64
	Tool    domain.ToolID
	Was     domain.Verdict
	WouldBe domain.Verdict
}

// maxSamples keeps the answer readable. A reader checks three; a list of five
// hundred is a second problem rather than evidence.
const maxSamples = 3

// Simulate replays a draft rule over recorded decisions.
func Simulate(draft domain.Policy, decisions []domain.RecordedDecision) Simulation {
	out := Simulation{
		Considered: len(decisions),
		ByVerdict:  map[domain.Verdict]int{},
	}
	readsArgs := ReadsArguments(draft)

	for _, d := range decisions {
		in := domain.PolicyInput{
			Tool: d.Tool, Effect: d.Effect, Agent: d.AgentID,
			Scope: d.Scope, Labels: d.Labels,
		}

		// Only where it would otherwise have applied: a rule that does not
		// cover this tool at all is not unknown, it is a no.
		if readsArgs && draft.Applies(in) {
			out.Unknown++
			continue
		}
		if !draft.Matches(in) {
			continue
		}

		verdict := draft.Effect.Verdict()
		out.Matched++
		out.ByVerdict[verdict]++
		if len(out.Samples) < maxSamples {
			out.Samples = append(out.Samples, Sample{
				RunID: d.RunID, Seq: d.Seq, Tool: d.Tool,
				Was: d.Verdict, WouldBe: verdict,
			})
		}
	}
	return out
}

// ReadsArguments reports whether any clause looks into the proposed arguments.
func ReadsArguments(p domain.Policy) bool {
	for _, c := range p.Conditions {
		if strings.HasPrefix(c.Field, "args.") {
			return true
		}
	}
	return false
}
