package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
What the planner reads of a rehearsal (NT-018 phase 4): the author's answer pairs with the
call it answers, though two steps stand between them, and the author's decision on a held
action reads as a person's decision. Results written before ForSeq existed still pair with
the step just before them.
*/
func rehearsalTranscript(t *testing.T, steps ...domain.Step) []Turn {
	t.Helper()
	for i := range steps {
		steps[i].RunID, steps[i].Seq = "run-1", int64(i+1)
	}
	turns, err := BuildTranscript(context.Background(), nil, steps)
	if err != nil {
		t.Fatalf("BuildTranscript: %v", err)
	}
	return turns
}

func TestTranscript_anAnswerForTheWorld_pairsWithTheCallItAnswers(t *testing.T) {
	t.Parallel()
	turns := rehearsalTranscript(t,
		domain.Step{Kind: domain.StepRunStarted, Payload: mustJSON(domain.RunStartedPayload{})},
		domain.Step{Kind: domain.StepToolCalled, Payload: mustJSON(domain.ToolCalledPayload{Tool: "crm.lookup"})},
		domain.Step{Kind: domain.StepWorldAwaited, Payload: mustJSON(domain.WorldAwaitedPayload{Tool: "crm.lookup", CallSeq: 2})},
		domain.Step{Kind: domain.StepWorldSupplied, Payload: mustJSON(domain.WorldSuppliedPayload{AtSeq: 3, Ref: "r", Digest: "d", By: "usr_ana"})},
		domain.Step{Kind: domain.StepToolReturned, Payload: mustJSON(domain.ToolReturnedPayload{Tool: "crm.lookup", ForSeq: 2})},
		domain.Step{Kind: domain.StepToolCalled, Payload: mustJSON(domain.ToolCalledPayload{Tool: "kb.search"})},
		domain.Step{Kind: domain.StepToolReturned, Payload: mustJSON(domain.ToolReturnedPayload{Tool: "kb.search"})},
	)
	calls, results := map[string]string{}, map[string]string{}
	for _, turn := range turns {
		switch turn.Kind {
		case TurnToolUse:
			calls[string(turn.Tool)] = turn.CallID
		case TurnToolResult:
			results[string(turn.Tool)] = turn.CallID
		}
	}
	for _, tool := range []string{"crm.lookup", "kb.search"} {
		if calls[tool] == "" || calls[tool] != results[tool] {
			t.Errorf("%s: call %q, result %q; want them paired", tool, calls[tool], results[tool])
		}
	}
}

func TestTranscript_theAuthorsDecision_readsAsAPersonsDecision(t *testing.T) {
	t.Parallel()
	for approved, said := range map[bool]string{true: "approved", false: "refused"} {
		turns := rehearsalTranscript(t,
			domain.Step{Kind: domain.StepRunStarted, Payload: mustJSON(domain.RunStartedPayload{})},
			domain.Step{Kind: domain.StepApprovalRequested, Payload: mustJSON(domain.ApprovalRequestedPayload{Tool: "crm.refund"})},
			domain.Step{Kind: domain.StepRehearsalDecided, Payload: mustJSON(domain.RehearsalDecidedPayload{AtSeq: 2, Approved: approved, By: "usr_ana"})},
		)
		last := turns[len(turns)-1]
		if last.Kind != TurnNote || !strings.Contains(last.Text, "A human "+said) {
			t.Errorf("approved=%v: last turn %+v, want a person's %s", approved, last, said)
		}
	}
}
