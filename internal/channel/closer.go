package channel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

/*
Closing a card is not a decision and cannot become one.

It rewrites a message the platform already posted so that it stops offering an
answer to a question that has one. It reads state, writes none, and its worst
failure is a button that answers 409 — which is what happens today, everywhere.

A sweep and not a hook, and the reason is stronger than surviving a crash: a
decision taken in the console never reaches the inbound endpoint at all, so a
hook there would close the cards of decisions made in Slack and leave every
console decision's cards live. Reading state is indifferent to where the
decision was taken.
*/
type Closer struct {
	cards   Cards
	editor  Editors
	clock   func() time.Time
	baseURL string
	log     *slog.Logger
}

func NewCloser(cards Cards, editor Editors, clock func() time.Time, log *slog.Logger) *Closer {
	if log == nil {
		log = slog.Default()
	}
	return &Closer{cards: cards, editor: editor, clock: clock, log: log}
}

// WithBaseURL puts the link to the run back on the closed card. The buttons go;
// the way to read what happened does not.
func (c *Closer) WithBaseURL(base string) *Closer {
	c.baseURL = base
	return c
}

/*
Sweep rewrites the cards whose question has been answered.

A card that cannot be rewritten for a reason another pass would meet again —
the message was deleted, the conversation is gone — is marked closed anyway.
Left open it would be retried for ever and would starve the limit, so the
cards that could still be fixed never get looked at.
*/
func (c *Closer) Sweep(ctx context.Context, limit int) (int, error) {
	open, err := c.cards.Stale(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("channel: read open cards: %w", err)
	}

	closed, failures := 0, []error{}
	for _, card := range open {
		at := Placement{Channel: card.Channel, Conversation: card.PlacedIn, Ref: card.Ref}
		err := c.editor.Edit(ctx, at, c.answered(card))
		if err != nil && !degrades(err) {
			failures = append(failures, fmt.Errorf(
				"channel: close the card for %s in %s: %w", card.RunID, card.Conversation, err))
			continue
		}
		if err != nil {
			c.log.Warn("a card could not be rewritten and will not be tried again",
				"run", card.RunID, "conversation", card.Conversation, "err", err)
		}
		if err := c.cards.Closed(ctx, card, c.clock()); err != nil {
			failures = append(failures, err)
			continue
		}
		closed++
	}
	return closed, errors.Join(failures...)
}

// answered is the card as it reads once the question is settled: the same
// facts, what happened to them, and no buttons.
func (c *Closer) answered(card Card) Message {
	m := Message{
		Event: card.Event, RunID: card.RunID, Agent: card.AgentID,
		Scope: card.Scope, Reason: card.Reason, Tool: card.Tool,
		Outcome: card.Outcome, DecidedBy: card.DecidedBy,
	}
	if c.baseURL != "" {
		m.Link = fmt.Sprintf("%s/runs/%s", c.baseURL, card.RunID)
	}
	return m
}
