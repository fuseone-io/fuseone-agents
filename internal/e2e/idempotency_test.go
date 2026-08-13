package e2e_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
The same effect does not happen twice (PRD DE-16), and taint reaches the write
however far it travels from the read that carried it (PRD SE-05).

Both are covered at the unit level against a fake tool layer. Neither was
covered against a real server, which is the only place the distinction between
"the platform decided not to call it again" and "it was not called again"
actually exists.
*/

func TestResume_afterTheRunFinished_theWriteIsNotMadeAgain(t *testing.T) {
	eachLedger(t, "advancing a finished run causes no second effect", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, writeThenAnswer)
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.note", Effect: domain.EffectWrite,
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.gate.allow("crm.note")

		p.open(t, "run-idem-1")
		if state := p.settle(t, "run-idem-1"); state.Phase != engine.PhaseFinished {
			t.Fatalf("phase = %v, want finished", state.Phase)
		}
		if got := len(p.server.Notes()); got != 1 {
			t.Fatalf("the server saw %d notes, want 1", got)
		}

		// A worker that claims a finished run — a lease that expired while the
		// last step was being written, a queue that handed it out twice — must
		// not repeat what it did. The ledger is the only memory it has.
		p.settle(t, "run-idem-1")

		if got := len(p.server.Notes()); got != 1 {
			t.Errorf("the server saw %d notes after a second pass, want the one from before", got)
		}
	})
}

// readThenWriteLater puts several steps between the untrusted read and the
// write that derives from it, so the taint has to survive them.
func readThenWriteLater(turn int) chatReply {
	switch turn {
	case 0:
		return chatReply{
			Tool: "crm.lookup", Args: `{"email":"cliente@exemplo.com.br"}`,
			PromptTokens: 1200, CompletionTokens: 80,
		}
	case 1:
		return chatReply{
			Tool: "crm.lookup", Args: `{"email":"outro@exemplo.com.br"}`,
			PromptTokens: 1200, CompletionTokens: 80,
		}
	}
	return chatReply{
		Tool: "crm.note", Args: `{"text":"conforme o cadastro"}`,
		PromptTokens: 1300, CompletionTokens: 40,
	}
}

func TestTaint_survivesTheStepsBetweenTheReadAndTheWrite(t *testing.T) {
	eachLedger(t, "taint from an early read still stops a later write", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, readThenWriteLater)
		for tool, effect := range map[domain.ToolID]domain.Effect{
			"crm.lookup": domain.EffectRead, "crm.note": domain.EffectWrite,
		} {
			if err := p.catalog.Classify(domain.ToolClassification{
				Tool: tool, Effect: effect, Untrusted: tool == "crm.lookup",
			}); err != nil {
				t.Fatalf("classify %s: %v", tool, err)
			}
		}
		// A written exception for the write, so what stops it can only be the
		// taint. Without this the built-in floor would stop it anyway and the
		// test would pass while proving nothing.
		p.gate.allow("crm.note")

		p.open(t, "run-taint-1")
		state := p.settle(t, "run-taint-1")

		if state.Phase != engine.PhaseAwaitingApproval {
			t.Fatalf("phase = %v, want it stopped for a person; steps: %v",
				state.Phase, kindsOf(p.steps(t, "run-taint-1")))
		}
		if got := len(p.server.Notes()); got != 0 {
			t.Errorf("the server saw %d notes; the write reached it despite the taint", got)
		}
	})
}
