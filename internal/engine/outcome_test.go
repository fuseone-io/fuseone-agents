package engine

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

const answer = "Refunded R$ 88,21 to Maria Silva, rua das Acacias 14."

// The model's closing answer restates whatever the agent read on the way, and
// run_steps has no UPDATE and no DELETE. Written into the step it is personal
// data an erasure request cannot reach — which is the one promise a governed
// platform cannot afford to break.
func TestRun_finished_doesNotWriteTheModelsAnswerIntoTheChain(t *testing.T) {
	t.Parallel()

	h := newHarness(t, Proposal{Done: true, Outcome: answer})
	if _, err := h.runner.Advance(context.Background(), h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	steps, err := h.ledger.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, step := range steps {
		if bytes.Contains(step.Payload, []byte("Maria Silva")) {
			t.Fatalf("step %s carries the answer verbatim: %s", step.Kind, step.Payload)
		}
	}
}

func TestRun_finished_putsTheAnswerWhereErasureReachesIt(t *testing.T) {
	t.Parallel()

	h := newHarness(t, Proposal{Done: true, Outcome: answer})
	if _, err := h.runner.Advance(context.Background(), h.start(t, generousBudget())); err != nil {
		t.Fatalf("Advance: %v", err)
	}

	var p domain.RunFinishedPayload
	if err := h.payloadOf(t, domain.StepRunFinished, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.OutcomeRef == "" || p.OutcomeDigest == "" {
		t.Fatal("the step names no reference and no digest for the answer")
	}

	stored, err := h.content.Get(context.Background(), p.OutcomeRef)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(stored) != answer {
		t.Errorf("stored %q, not the answer", stored)
	}

	// The point of the move: the same erasure that reaches a tool's arguments
	// now reaches the model's answer.
	if _, err := h.content.Erase(context.Background(), "run-1", "subject request"); err != nil {
		t.Fatalf("Erase: %v", err)
	}
	if _, err := h.content.Get(context.Background(), p.OutcomeRef); err == nil {
		t.Error("the answer survived an erasure of its run")
	}
}

// A run recorded before the move keeps its answer inline, and the chain is
// immutable, so it stays there. Reading it must still work: rewriting old
// steps to tidy this up would break the property the whole product rests on.
func TestOutcomeOf_runFinishedBeforeTheMove_stillReads(t *testing.T) {
	t.Parallel()

	got, err := OutcomeOf(context.Background(), nil, domain.RunFinishedPayload{Outcome: answer})
	if err != nil {
		t.Fatalf("OutcomeOf: %v", err)
	}
	if got != answer {
		t.Errorf("read %q from an old run, not its answer", got)
	}
}

func TestOutcomeOf_answerErased_saysSoRatherThanReadingEmpty(t *testing.T) {
	t.Parallel()

	store := NewMemoryContent()
	ref, err := store.Put(context.Background(), "run-1", 2, []byte(answer))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := store.Erase(context.Background(), "run-1", "retention"); err != nil {
		t.Fatalf("Erase: %v", err)
	}

	// An erased answer and an agent that finished silently are different
	// facts. Rendering both as "" would let a screen report the second when
	// the first happened.
	got, err := OutcomeOf(context.Background(), store, domain.RunFinishedPayload{OutcomeRef: ref})
	if err == nil {
		t.Fatalf("read %q from erased content instead of saying it was erased", got)
	}
	if !strings.Contains(err.Error(), "erased") {
		t.Errorf("error %q does not name erasure", err)
	}
}
