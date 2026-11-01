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

// byEvent is the position of a run an event opened, carrying chain.
func byEvent(chain *domain.EventChain) domain.ChainPosition {
	return domain.ChainPosition{Trigger: domain.TriggerEvent, Chain: chain}
}

// finishAt advances a run opened at position to its finish and returns the
// closing payload.
func finishAt(t *testing.T, position domain.ChainPosition, publishes bool) domain.RunFinishedPayload {
	t.Helper()
	ctx := context.Background()
	h := newHarness(t, Proposal{Done: true, Outcome: "done"})
	start := h.start(t, generousBudget())
	start.PublishesEvents = publishes
	if _, err := h.ledger.Append(ctx, domain.Step{
		RunID: start.RunID, Kind: domain.StepRunStarted,
		Scope: start.Scope, AgentID: start.AgentID,
		VersionID: start.VersionID, OnBehalfOf: start.OnBehalfOf,
		Payload: mustJSON(domain.RunStartedPayload{Trigger: position.Trigger, Chain: position.Chain}),
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

	finished := finishAt(t, byEvent(&domain.EventChain{Root: "run-root", Hop: domain.MaxEventHops}), true)

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

	cases := map[string]domain.ChainPosition{
		"no event opened it": {Trigger: "cron"},
		"one hop short":      byEvent(&domain.EventChain{Root: "run-root", Hop: domain.MaxEventHops - 1}),
	}
	for name, position := range cases {
		if got := finishAt(t, position, true).EventsWithheld; got != nil {
			t.Errorf("%s: events withheld = %+v, want none", name, got)
		}
	}
}

// Opened by an event with no chain — by an older dispatcher during a rolling
// update — the run cannot know how far it stands from its root. Read as a
// root it would restart the count, so it withholds, and says why.
func TestAdvance_anEventOpenedRunWithNoChain_recordsTheChainAsUnknown(t *testing.T) {
	t.Parallel()

	finished := finishAt(t, byEvent(nil), true)

	want := domain.EventsWithheld{Reason: domain.ReasonEventChainUnknown, Limit: domain.MaxEventHops}
	if finished.EventsWithheld == nil || *finished.EventsWithheld != want {
		t.Fatalf("events withheld = %+v, want %+v", finished.EventsWithheld, want)
	}
}

// An agent that declares no events has nothing to withhold, and a line
// saying otherwise on its trail would be noise.
func TestAdvance_aRunAtTheHopLimitThatPublishesNothing_recordsNothing(t *testing.T) {
	t.Parallel()

	chain := &domain.EventChain{Root: "run-root", Hop: domain.MaxEventHops}
	if got := finishAt(t, byEvent(chain), false).EventsWithheld; got != nil {
		t.Fatalf("events withheld = %+v for an agent that publishes none", got)
	}
}
