package e2e_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
Parking is a pause, and the product's claim about it is precise: raise the
ceiling and the run continues from the exact step it stopped at, without
repeating the effect it already caused (PRD FO-04, NF-14).

Every part of that is testable and none of it was tested. A run that resumed by
starting over would repeat a write against a system that already changed, which
is the failure a ledger-based platform exists to make impossible.
*/

// ceiling is the scope budget an operator raises. Settable, because the whole
// point is what happens when somebody changes it between two turns.
type ceiling struct {
	mu     sync.Mutex
	budget domain.Budget
}

func (c *ceiling) Resolve(context.Context, domain.Scope) (domain.Budget, domain.Period, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.budget, domain.PeriodDaily, nil
}

func (c *ceiling) SpentSince(context.Context, domain.Scope, time.Time) (domain.Consumption, error) {
	return domain.Consumption{}, nil
}

func (c *ceiling) raise(to domain.Budget) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.budget = to
}

// twoWritesThenAnswer needs more room than the ceiling allows, so the run gets
// one write done and stops with the second still to make.
func twoWritesThenAnswer(turn int) chatReply {
	switch turn {
	case 0:
		return chatReply{
			Tool: "crm.note", Args: `{"text":"primeira nota"}`,
			PromptTokens: 900, CompletionTokens: 30,
		}
	case 1:
		return chatReply{
			Tool: "crm.note", Args: `{"text":"segunda nota"}`,
			PromptTokens: 950, CompletionTokens: 30,
		}
	}
	return chatReply{Text: "Notas registradas.", PromptTokens: 1000, CompletionTokens: 20}
}

func TestCeiling_raised_resumesWithoutRepeatingTheWrite(t *testing.T) {
	eachLedger(t, "raising a ceiling continues the run rather than restarting it", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, twoWritesThenAnswer)
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.note", Effect: domain.EffectWrite,
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.gate.allow("crm.note")

		// Room for the write and nothing after it.
		limit := &ceiling{budget: domain.Budget{Micros: 1_000_000, ToolCalls: 1, Steps: 40}}
		p.worker = p.worker.WithCeilings(limit)

		p.open(t, "run-ceiling-1")
		if state := p.settle(t, "run-ceiling-1"); state.Phase != engine.PhaseParked {
			t.Fatalf("phase = %v, want it parked at the ceiling; steps: %v",
				state.Phase, kindsOf(p.steps(t, "run-ceiling-1")))
		}
		if got := len(p.server.Notes()); got != 1 {
			t.Fatalf("the server saw %d notes before the ceiling, want 1", got)
		}
		at := p.state(t, "run-ceiling-1").Seq

		limit.raise(domain.Budget{Micros: 1_000_000, ToolCalls: 20, Steps: 40})
		// Raising the ceiling is half of it. Somebody has to say they raised
		// it: a ceiling lifted across a company must not silently restart
		// every run that ever hit it.
		p.resume(t, "run-ceiling-1")
		state := p.settle(t, "run-ceiling-1")

		// The claim, in two halves. It carried on from where it stopped, and
		// the write it had already made did not happen again.
		if state.Seq <= at {
			t.Errorf("Seq = %d, want the run to have moved past %d", state.Seq, at)
		}
		if got := len(p.server.Notes()); got != 1 {
			t.Errorf("the server saw %d notes after the resume, want the one from before", got)
		}
		if err := store.Verify(t.Context(), "run-ceiling-1"); err != nil {
			t.Errorf("Verify: %v", err)
		}
	})
}

// resume is a person returning a parked run to the queue, the way the API does.
func (p *platform) resume(t *testing.T, runID domain.RunID) {
	t.Helper()

	payload, err := json.Marshal(domain.ResumedPayload{By: "ana", Note: "teto erguido"})
	if err != nil {
		t.Fatalf("encode resume: %v", err)
	}
	if _, err := p.store.Append(t.Context(), domain.Step{
		RunID: runID, Kind: domain.StepResumed,
		Scope:     domain.Scope{Company: "acme", Area: "cx"},
		AgentID:   p.spec.ID,
		VersionID: p.spec.Version,
		At:        time.Now(),
		Payload:   payload,
	}); err != nil {
		t.Fatalf("resume run: %v", err)
	}
}
