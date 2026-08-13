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

// writeThenAnswer makes the note once and then reports done, which is the run
// an approver expects: they cleared one thing and it happened.
func writeThenAnswer(turn int) chatReply {
	if turn == 0 {
		return chatReply{
			Tool: "crm.note", Args: `{"text":"cobranca duplicada"}`,
			PromptTokens: 900, CompletionTokens: 30,
		}
	}
	return chatReply{Text: "Nota registrada.", PromptTokens: 950, CompletionTokens: 20}
}

func TestApproval_granted_executesTheCallThatWasApproved(t *testing.T) {
	eachLedger(t, "an approved write reaches the server", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, writeThenAnswer)
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.note", Effect: domain.EffectWrite,
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}

		// No written exception, so the built-in floor sends the write to a
		// person. That is the whole path this covers.
		p.open(t, "run-approval-1")
		if state := p.settle(t, "run-approval-1"); state.Phase != engine.PhaseAwaitingApproval {
			t.Fatalf("phase = %v, want it waiting for a person", state.Phase)
		}
		if len(p.server.Notes()) != 0 {
			t.Fatal("the write reached the server before anybody approved it")
		}

		p.decide(t, "run-approval-1", true)
		state := p.settle(t, "run-approval-1")

		// The assertion the platform did not have: a granted approval has to
		// reach the outside world. Without it the loop replans, the Gate asks
		// again, and the run circles until it parks.
		if got := p.server.Notes(); len(got) != 1 {
			t.Fatalf("the server saw %d notes, want the approved one; steps: %v",
				len(got), kindsOf(p.steps(t, "run-approval-1")))
		}
		if state.Phase != engine.PhaseFinished {
			t.Errorf("phase = %v, want the run to have carried on", state.Phase)
		}
	})
}

// decide records a person's answer the way the API does.
func (p *platform) decide(t *testing.T, runID domain.RunID, granted bool) {
	t.Helper()

	payload, err := json.Marshal(domain.ApprovalDecidedPayload{Approved: granted, By: "ana"})
	if err != nil {
		t.Fatalf("encode decision: %v", err)
	}
	if _, err := p.store.Append(t.Context(), domain.Step{
		RunID: runID, Kind: domain.StepApprovalDecided,
		Scope:     domain.Scope{Company: "acme", Area: "cx"},
		AgentID:   p.spec.ID,
		VersionID: p.spec.Version,
		At:        time.Now(),
		Payload:   payload,
	}); err != nil {
		t.Fatalf("decide approval: %v", err)
	}
}
