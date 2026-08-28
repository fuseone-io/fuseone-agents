package engine

import (
	"bytes"
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestAdvance_erasableContentStaysOutsideTheLedger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const (
		argsMarker   = "RETENTION-ARGS-ANA"
		resultMarker = "RETENTION-RESULT-RUA-14"
		answer       = "RETENTION-ANSWER-MARIA"
	)
	args := []byte(`{"customer":"` + argsMarker + `"}`)
	result := []byte(`{"address":"` + resultMarker + `"}`)
	h := newHarness(t,
		Proposal{Tool: "crm.lookup", Args: args},
		Proposal{Done: true, Outcome: answer},
	)
	h.tools.body = result
	start := h.start(t, generousBudget())

	for range 2 {
		if _, err := h.runner.Advance(ctx, start); err != nil {
			t.Fatalf("Advance: %v", err)
		}
	}

	steps, err := h.ledger.Read(ctx, start.RunID, domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, step := range steps {
		for _, marker := range []string{argsMarker, resultMarker, answer} {
			if bytes.Contains(step.Payload, []byte(marker)) {
				t.Fatalf("%s writes erasable content into the ledger: %s", step.Kind, step.Payload)
			}
		}
	}

	refs := contentReferences(t, h)
	want := map[string][]byte{"arguments": args, "result": result, "answer": []byte(answer)}
	for name, ref := range refs {
		got, err := h.content.Get(ctx, ref)
		if err != nil {
			t.Fatalf("read %s content: %v", name, err)
		}
		if !bytes.Equal(got, want[name]) {
			t.Errorf("%s content = %q, want %q", name, got, want[name])
		}
	}
	if _, err := h.content.Erase(ctx, string(start.RunID), "retention"); err != nil {
		t.Fatalf("Erase: %v", err)
	}
	for name, ref := range refs {
		if _, err := h.content.Get(ctx, ref); err == nil {
			t.Errorf("%s content survived erasure", name)
		}
	}
}

func contentReferences(t *testing.T, h *harness) map[string]string {
	t.Helper()
	var called domain.ToolCalledPayload
	if err := h.payloadOf(t, domain.StepToolCalled, &called); err != nil {
		t.Fatalf("tool_called: %v", err)
	}
	var returned domain.ToolReturnedPayload
	if err := h.payloadOf(t, domain.StepToolReturned, &returned); err != nil {
		t.Fatalf("tool_returned: %v", err)
	}
	var finished domain.RunFinishedPayload
	if err := h.payloadOf(t, domain.StepRunFinished, &finished); err != nil {
		t.Fatalf("run_finished: %v", err)
	}
	if called.ArgsRef == "" || called.ArgsDigest == "" ||
		returned.ResultRef == "" || returned.ResultDigest == "" ||
		finished.OutcomeRef == "" || finished.OutcomeDigest == "" {
		t.Fatalf("erasable content is not fully named by reference and digest")
	}
	return map[string]string{
		"arguments": called.ArgsRef,
		"result":    returned.ResultRef,
		"answer":    finished.OutcomeRef,
	}
}
