package simulate_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

// A rehearsal whose trail holds a link (#204) is reported like any other: the
// link is a record of the trail, not an act of the agent, so it adds a step
// to the row and nothing to what the case did or why it stopped.
func TestFold_aSimulatedRunWithAWorkItemLink_isReported(t *testing.T) {
	t.Parallel()

	got, err := simulate.Fold(run(t,
		started(),
		entry{kind: domain.StepWorkItemClaimed, payload: domain.WorkItemClaimedPayload{
			Ref: "github:fuseone-io/fuseone-agents#204",
		}},
		entry{kind: domain.StepToolReturned, payload: domain.ToolReturnedPayload{ResultRef: "content:pr"}},
		entry{kind: domain.StepWorkItemLinked, payload: domain.WorkItemLinkedPayload{
			Alias: "github:fuseone-io/fuseone-agents#240", Root: "github:fuseone-io/fuseone-agents#204",
			Proof: domain.WorkItemLinkProof{Kind: domain.StepToolReturned, Seq: 3},
		}},
		entry{kind: domain.StepRunFinished, payload: domain.RunFinishedPayload{Outcome: "opened the pull request"}},
	))
	if err != nil {
		t.Fatalf("Fold(a rehearsal with a link) = %v, want its row", err)
	}
	if got.Steps != 5 || got.Settled != simulate.SettledFinished || len(got.Acted) != 0 {
		t.Fatalf("case = %+v, want 5 steps, finished, nothing acted on by the link", got)
	}
}
