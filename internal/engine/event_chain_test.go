package engine

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A run at the end of an event chain says so on its closing step (#203). The
dispatcher will open nothing from it; without this record the next agent
simply never starts, and the trail has a gap somebody has to explain.
*/

// finishAt advances a run an event opened at chain to its finish and returns
// the closing payload.
func finishAt(t *testing.T, chain *domain.EventChain, publishes bool) domain.RunFinishedPayload {
	t.Helper()
	ctx := context.Background()
	h := newHarness(t, Proposal{Done: true, Outcome: "done"})
	start := h.start(t, generousBudget())
	start.PublishesEvents = publishes
	if _, err := h.ledger.Append(ctx, domain.Step{
		RunID: start.RunID, Kind: domain.StepRunStarted,
		Scope: start.Scope, AgentID: start.AgentID,
		VersionID: start.VersionID, OnBehalfOf: start.OnBehalfOf,
		Payload: mustJSON(domain.RunStartedPayload{Trigger: "event", Chain: chain}),
	}); err != nil {
		t.Fatalf("open run: %v", err)
	}
	if _, err := h.runner.Advance(ctx, start); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	var finished domain.RunFinishedPayload
	if err := h.payloadOf(t, domain.StepRunFinished, &finished); err != nil {
		t.Fatalf("payload: %v", err)
	}
	return finished
}

func TestAdvance_aRunAtTheHopLimitThatPublishes_recordsItsEventsWithheld(t *testing.T) {
	t.Parallel()

	finished := finishAt(t, &domain.EventChain{Root: "run-root", Hop: domain.MaxEventHops}, true)

	want := domain.EventsWithheld{
		Reason: domain.ReasonEventHopLimit, Root: "run-root",
		Hop: domain.MaxEventHops, Limit: domain.MaxEventHops,
	}
	if finished.EventsWithheld == nil || *finished.EventsWithheld != want {
		t.Fatalf("events withheld = %+v, want %+v", finished.EventsWithheld, want)
	}
}

func TestAdvance_aRunUnderTheHopLimit_withholdsNothing(t *testing.T) {
	t.Parallel()

	cases := map[string]*domain.EventChain{
		"no event opened it": nil,
		"one hop short":      {Root: "run-root", Hop: domain.MaxEventHops - 1},
	}
	for name, chain := range cases {
		if got := finishAt(t, chain, true).EventsWithheld; got != nil {
			t.Errorf("%s: events withheld = %+v, want none", name, got)
		}
	}
}

// An agent that declares no events has nothing to withhold, and a line
// saying otherwise on its trail would be noise.
func TestAdvance_aRunAtTheHopLimitThatPublishesNothing_recordsNothing(t *testing.T) {
	t.Parallel()

	chain := &domain.EventChain{Root: "run-root", Hop: domain.MaxEventHops}
	if got := finishAt(t, chain, false).EventsWithheld; got != nil {
		t.Fatalf("events withheld = %+v for an agent that publishes none", got)
	}
}
