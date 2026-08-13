package e2e_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
A model provider going down pauses runs resumably, loses no state and
duplicates no effect (PRD NF-11).

The failure mode this guards against is the expensive one: an outage that
makes the platform retry a write it already made. The worker cannot know
whether the provider died before or after the effect, so the answer has to
come from the ledger rather than from the attempt.
*/
func TestProviderDown_parksWithoutRepeatingTheWriteItAlreadyMade(t *testing.T) {
	eachLedger(t, "an outage pauses the run and repeats nothing", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, twoWritesThenAnswer)
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.note", Effect: domain.EffectWrite,
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.gate.allow("crm.note")

		p.open(t, "run-outage-1")
		p.settle(t, "run-outage-1")
		if got := len(p.server.Notes()); got == 0 {
			t.Fatal("the run wrote nothing before the outage")
		}
		wrote := len(p.server.Notes())

		// The provider goes down between two turns, which is where an outage
		// actually lands: after an effect and before the next decision.
		p.model.fail()
		p.open(t, "run-outage-2")
		state := p.settle(t, "run-outage-2")

		if state.Phase != engine.PhaseParked {
			t.Errorf("phase = %v, want the run paused rather than failed", state.Phase)
		}
		// Parked, not ended: the run has to be able to carry on once the
		// provider is back, which is what makes an outage survivable.
		if state.Terminal() {
			t.Error("the outage ended the run; a provider coming back would not help it")
		}
		if got := len(p.server.Notes()); got != wrote {
			t.Errorf("the server saw %d notes, want the %d from before the outage", got, wrote)
		}

		// And the trail says what happened, rather than stopping mid-turn.
		if err := store.Verify(t.Context(), "run-outage-2"); err != nil {
			t.Errorf("Verify: %v", err)
		}
	})
}

func TestProviderRestored_theParkedRunCarriesOn(t *testing.T) {
	eachLedger(t, "the run continues once the provider is back", func(t *testing.T, store Store) {
		p := newPlatform(t, store, agentFull, writeThenAnswer)
		if err := p.catalog.Classify(domain.ToolClassification{
			Tool: "crm.note", Effect: domain.EffectWrite,
		}); err != nil {
			t.Fatalf("classify: %v", err)
		}
		p.gate.allow("crm.note")

		p.model.fail()
		p.open(t, "run-outage-3")
		if state := p.settle(t, "run-outage-3"); state.Phase != engine.PhaseParked {
			t.Fatalf("phase = %v, want it parked by the outage", state.Phase)
		}

		p.model.restore()
		p.resume(t, "run-outage-3")
		state := p.settle(t, "run-outage-3")

		if state.Phase != engine.PhaseFinished {
			t.Errorf("phase = %v, want the run to have finished; steps: %v",
				state.Phase, kindsOf(p.steps(t, "run-outage-3")))
		}
		if got := len(p.server.Notes()); got != 1 {
			t.Errorf("the server saw %d notes, want exactly the one the run makes", got)
		}
	})
}
