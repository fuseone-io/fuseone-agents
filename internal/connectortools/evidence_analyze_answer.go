package connectortools

import (
	"cmp"
	"slices"

	"github.com/fuseone/agents/internal/engine"
)

// answer puts the base before the conclusion: window, scope and counts, then the
// coverage, and only then the outcome.
func (t *evidenceTally) answer(call engine.Call, state evidenceState) map[string]any {
	answer := map[string]any{
		"operation": "evidence.analyze", "instance": state.Instance, "query": state.Query,
		"scope": call.Scope.String(), "params": state.Params, "collectedWindow": state.Window,
		"events": t.events, "duplicates": t.duplicates, "unmeasured": t.unmeasured,
		"coverage": state.Coverage, "control": state.Control,
		"outcome": evidenceOutcome(state, t.events, t.unmeasured),
	}
	if state.Reason != "" {
		answer["reason"] = state.Reason
	}
	if t.cut.active {
		answer["window"] = t.args.Window
		answer["outsideWindow"], answer["undated"] = t.outside, t.undated
	}
	if len(t.sums) > 0 {
		answer["sums"] = t.sums
	}
	if len(t.groups) > 0 {
		answer["groups"] = t.renderGroups()
	}
	return answer
}

func (t *evidenceTally) renderGroups() []map[string]any {
	top := t.args.Top
	if top == 0 {
		top = defaultEvidenceTop
	}
	out := make([]map[string]any, 0, len(t.args.GroupBy))
	for _, field := range t.args.GroupBy {
		group := t.groups[field]
		rendered := map[string]any{
			"field": field, "distinct": len(group.buckets), "missing": group.missing,
			"distinctIsAMinimum": group.capped,
			"values":             t.rankBuckets(group, top),
		}
		out = append(out, rendered)
	}
	return out
}

// rankBuckets orders by the first sum asked for, because volume is the question
// a sum is there to answer, then by count, then by value so the answer is stable.
func (t *evidenceTally) rankBuckets(group *evidenceGroup, top int) []*evidenceBucket {
	buckets := make([]*evidenceBucket, 0, len(group.buckets))
	for _, bucket := range group.buckets {
		buckets = append(buckets, bucket)
	}
	var first string
	if len(t.args.Sum) > 0 {
		first = t.args.Sum[0]
	}
	slices.SortFunc(buckets, func(a, b *evidenceBucket) int {
		return cmp.Or(
			cmp.Compare(b.Sums[first], a.Sums[first]),
			cmp.Compare(b.Count, a.Count),
			cmp.Compare(a.Value, b.Value),
			cmp.Compare(a.Digest, b.Digest),
		)
	})
	return buckets[:min(top, len(buckets))]
}
