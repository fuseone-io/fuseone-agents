package e2e_test

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
A tool that returns more than the installation stores (PRD DE-03).

Every stack has one — a report, an export, a log query — and until now nothing
bounded it: whatever came back went into a row, whole, having first been held
in memory. What matters is not only that it is bounded but that the run knows,
because a model handed half a document with no notice reasons over it as though
it were whole.
*/

// dumpsTooMuch asks for far more than the limit, then answers.
func dumpsTooMuch(turn int) chatReply {
	if turn == 0 {
		return chatReply{
			// More than the default a store keeps, so the test exercises the
			// number an installation actually runs with rather than one the
			// harness invented.
			Tool: "crm.dump", Args: `{"bytes":2000000}`,
			PromptTokens: 900, CompletionTokens: 30,
		}
	}
	return chatReply{Text: "Relatório lido.", PromptTokens: 950, CompletionTokens: 20}
}

func TestContent_aToolThatReturnsTooMuch_isKeptInPartAndSaysSo(t *testing.T) {
	eachLedger(t, "an oversized result is bounded and the run is told", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, dumpsTooMuch)
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.dump", Effect: domain.EffectRead,
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}

		p.open(t, "run-limit-1")
		if state := p.settle(t, "run-limit-1"); state.Phase != engine.PhaseFinished {
			t.Fatalf("phase = %v, want the run to have carried on", state.Phase)
		}

		// What the model was actually shown on the turn after the call. The
		// point of the notice is that it reaches here.
		var transcript string
		for _, req := range p.model.requests() {
			for _, message := range req.Messages {
				transcript += message.Content
			}
		}
		if !strings.Contains(transcript, "truncated") {
			t.Error("the model was handed a partial result with nothing saying so")
		}
		if !strings.Contains(transcript, "2000") {
			t.Error("the notice does not say how much the tool actually returned")
		}
	})
}
