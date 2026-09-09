package channel_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
)

/*
A card stops offering an answer once the question has one.

Somebody decides in one place and every other copy is still asking. Pressing
one of those is refused with a conflict, which is correct and leaves the card
wrong: it says a run is waiting when it is not, in a room where other people
are reading it.

Read as state and not pushed from the decision, because most decisions never
pass through a channel at all. A hook on the inbound path would close the cards
of decisions made in Slack and leave every console decision's cards live.
*/
func TestCloserSweep_aDecidedRun_rewritesTheCardsAndSaysWhoAnswered(t *testing.T) {
	cards := &openCards{cards: []channel.Card{
		card("C07-ops", "C07-ops", channel.OutcomeApproved, "usr_ana"),
		card("U-bruno", "D-bruno", channel.OutcomeApproved, "usr_ana"),
	}}
	edits := &editRecorder{}

	closed, err := channel.NewCloser(cards, edits, func() time.Time { return noon }, nil).
		Sweep(context.Background(), 10)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if closed != 2 {
		t.Fatalf("closed %d cards, want both", closed)
	}
	// The private one is rewritten where Slack put it, not where it was sent.
	// chat.postMessage takes a person; chat.update does not.
	if edits.at[1].Conversation != "D-bruno" {
		t.Errorf("edited %q, want the conversation the vendor named", edits.at[1].Conversation)
	}
	for _, m := range edits.messages {
		if m.Outcome != channel.OutcomeApproved || m.DecidedBy != "usr_ana" {
			t.Errorf("message = %+v, want it to say who answered", m)
		}
	}
	if len(cards.closed) != 2 {
		t.Errorf("recorded %d closed, want both", len(cards.closed))
	}
}

/*
A card nothing can rewrite is closed anyway.

The message was deleted, or the conversation is gone. Left open it is retried
on every sweep for ever, and the budget it spends is budget the cards that
could still be fixed never get.
*/
func TestCloserSweep_aCardThatCannotBeRewritten_isNotTriedForever(t *testing.T) {
	cards := &openCards{cards: []channel.Card{
		card("C07-ops", "C07-ops", channel.OutcomeRefused, "usr_ana"),
	}}
	edits := &editRecorder{fail: channel.NewError(
		channel.CodeConversationUnavailable, "slack: refused: message_not_found")}

	if _, err := channel.NewCloser(cards, edits, func() time.Time { return noon }, nil).
		Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(cards.closed) != 1 {
		t.Error("a card nothing can rewrite was left open for the next sweep")
	}
}

// A refusal another sweep could survive leaves the card open. A rate limit is
// not a message that has gone away.
func TestCloserSweep_aTransientRefusal_leavesTheCardForTheNextSweep(t *testing.T) {
	cards := &openCards{cards: []channel.Card{
		card("C07-ops", "C07-ops", channel.OutcomeApproved, "usr_ana"),
	}}
	edits := &editRecorder{fail: channel.NewError(channel.CodeRateLimited, "slack: rate limited")}

	if _, err := channel.NewCloser(cards, edits, func() time.Time { return noon }, nil).
		Sweep(context.Background(), 10); err == nil {
		t.Fatal("Sweep reported success with a card it could not rewrite")
	}
	if len(cards.closed) != 0 {
		t.Error("a card was closed after a refusal another sweep could survive")
	}
}

// A run that moved on with nobody deciding says exactly that. Calling it
// refused would put a decision in somebody's mouth that nobody made.
func TestCloserSweep_aRunThatMovedOn_claimsNoDecision(t *testing.T) {
	cards := &openCards{cards: []channel.Card{
		card("C07-ops", "C07-ops", channel.OutcomeMovedOn, ""),
	}}
	edits := &editRecorder{}

	if _, err := channel.NewCloser(cards, edits, func() time.Time { return noon }, nil).
		Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if edits.messages[0].Outcome != channel.OutcomeMovedOn {
		t.Errorf("outcome = %q, want it to claim no decision", edits.messages[0].Outcome)
	}
}

func card(conversation, placedIn string, outcome channel.Outcome, by string) channel.Card {
	return channel.Card{
		Announcement: channel.Announcement{
			RunID: "run-1", Event: channel.EventParked, AtSeq: 4,
			Channel: "acme-slack", Conversation: conversation,
		},
		PlacedIn: placedIn, Ref: "1786.1", AgentID: "triage",
		Scope:   domain.Scope{Company: "acme", Area: "ops"},
		Tool:    "erp.transfer",
		Outcome: outcome, DecidedBy: by,
	}
}

type openCards struct {
	cards  []channel.Card
	closed []channel.Card
	err    error
}

func (o *openCards) Stale(context.Context, int) ([]channel.Card, error) {
	return o.cards, o.err
}

func (o *openCards) Closed(_ context.Context, c channel.Card, _ time.Time) error {
	o.closed = append(o.closed, c)
	return nil
}

type editRecorder struct {
	at       []channel.Placement
	messages []channel.Message
	fail     error
}

func (e *editRecorder) Edit(_ context.Context, at channel.Placement, m channel.Message) error {
	if e.fail != nil {
		return e.fail
	}
	e.at = append(e.at, at)
	e.messages = append(e.messages, m)
	return nil
}
