package e2e_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// Abandoning a run: the whole path, with nothing stubbed.
//
// The point of these is the last assertion in each. A ledger full of
// "compensated" steps proves the platform decided to undo something; only the
// MCP server proves the note is actually gone.

// writeThenStall makes the note and then keeps proposing it, which is what a
// model does when the world stopped matching its plan. The run gets somewhere
// real and then cannot finish — exactly the run somebody abandons.
func writeThenStall(int) chatReply {
	return chatReply{
		Tool: "crm.note", Args: `{"text":"cobranca duplicada"}`,
		PromptTokens: 900, CompletionTokens: 30,
	}
}

func TestAbandon_theUndoReachesTheServer(t *testing.T) {
	eachLedger(t, "abandoning a run removes what it wrote", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, writeThenStall)

		// The Curator rules on what the tool does and on what takes it back,
		// in one act: they are one judgement.
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.note", Effect: domain.EffectWrite, CompensatedBy: "crm.unnote",
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.gate.allow("crm.note")

		p.open(t, "run-abandon-1")
		if state := p.settle(t, "run-abandon-1"); state.Phase != engine.PhaseParked {
			t.Fatalf("phase = %v, want the run stuck and parked", state.Phase)
		}
		if len(p.server.Notes()) == 0 {
			t.Fatal("the run wrote nothing; there is nothing to undo")
		}

		p.abandon(t, "run-abandon-1")
		state := p.settle(t, "run-abandon-1")

		// The permission the undo stood on is the one that let the note
		// happen: crm.unnote is not in the agent's pack, and never was.
		if got := p.server.Unnotes(); len(got) != 1 {
			t.Fatalf("the server saw %d removals, want exactly 1; steps: %v",
				len(got), kindsOf(p.steps(t, "run-abandon-1")))
		}
		if state.Phase != engine.PhaseFailed {
			t.Errorf("phase = %v, want failed; an abandoned run did not finish", state.Phase)
		}
		if err := store.Verify(t.Context(), "run-abandon-1"); err != nil {
			t.Errorf("Verify: %v", err)
		}
	})
}

func TestAbandon_withNothingToUndoIt_theActStandsAndTheRunStillEnds(t *testing.T) {
	eachLedger(t, "an act nobody can undo does not hold the run open", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, writeThenStall)

		// Classified, but nobody said what takes it back. The honest outcome
		// is a run that ended with something still standing in the world, not
		// a run that reports itself cleanly undone.
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.note", Effect: domain.EffectWrite,
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.gate.allow("crm.note")

		p.open(t, "run-abandon-2")
		p.settle(t, "run-abandon-2")
		p.abandon(t, "run-abandon-2")
		state := p.settle(t, "run-abandon-2")

		if len(p.server.Unnotes()) != 0 {
			t.Errorf("the server saw a removal nobody declared: %v", p.server.Unnotes())
		}
		if state.Phase != engine.PhaseFailed {
			t.Errorf("phase = %v, want the run ended anyway", state.Phase)
		}
	})
}

// abandon is a person deciding the run is over. It records the decision and
// returns; the undoing is a worker's job, because a tool call takes as long as
// it takes.
func (p *platform) abandon(t *testing.T, runID domain.RunID) {
	t.Helper()

	payload, err := json.Marshal(domain.AbandonedPayload{
		By: "ana", Reason: "duplicate charge", Compensate: true,
	})
	if err != nil {
		t.Fatalf("encode abandonment: %v", err)
	}
	if _, err := p.store.Append(t.Context(), domain.Step{
		RunID: runID, Kind: domain.StepAbandoned,
		Scope:     domain.Scope{Company: "acme", Area: "cx"},
		AgentID:   p.spec.ID,
		VersionID: p.spec.Version,
		At:        time.Now(),
		Payload:   payload,
	}); err != nil {
		t.Fatalf("abandon run: %v", err)
	}
}
