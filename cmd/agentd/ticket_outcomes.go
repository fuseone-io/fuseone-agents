package main

import (
	"context"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/ticket"
	"github.com/fuseone/agents/internal/worker"
)

// Separate from model answers because governed effects are rendered by
// connector code from a safe projection. Neither loop delays the other when
// Slack or content retention is unhealthy.
const ticketOutcomeAnswer = 10 * time.Second

func consumeTicketOutcomes(
	ctx context.Context, p *workerParts, owner string, metrics *worker.MetricsRegistry,
	tickets ticket.OutcomeNotices, answers channel.Answers,
) {
	outcomes := channel.NewTicketOutcomeConsumer(
		tickets, p.content, answers, ticketOutcomeRenderers{}, owner,
	)
	go channelSweepLoop(ctx, ticketOutcomeAnswer, channel.MetricTaskTicketOutcomes,
		"ticket outcomes delivered", metrics, func() (int, error) {
			return outcomes.Sweep(ctx, askLease, 20)
		})
}

// ticketReviewOpening is how often tickets configured with a room of their own
// get the thread they are reviewed in. Short, because nothing about the ticket
// can be decided until that thread exists: its cards wait for it rather than
// falling back to the thread the person who asked is reading.
const ticketReviewOpening = 5 * time.Second

func consumeTicketReviewRooms(
	ctx context.Context, owner string, metrics *worker.MetricsRegistry,
	rooms ticket.ReviewRooms, opener channel.ReviewOpener,
) {
	reviews := channel.NewTicketReviewConsumer(rooms, opener, owner)
	go channelSweepLoop(ctx, ticketReviewOpening, channel.MetricTaskTicketReviews,
		"ticket review rooms opened", metrics, func() (int, error) {
			return reviews.Sweep(ctx, askLease, 20)
		})
}

/*
ticketOutcomeRenderers picks who writes the closing message.

By the projection rather than by the tool that produced it: the consumer
claims an outcome, not a call, and the bytes it holds are the only thing that
still says what the effect was. An answer a person approved is published as
those words; a connector's effect is described by the connector.
*/
type ticketOutcomeRenderers struct{}

func (ticketOutcomeRenderers) RenderTicketOutcome(
	phase ticket.Phase, raw []byte,
) (string, error) {
	if message, err := (connectortools.TicketAnswerRenderer{}).RenderTicketOutcome(phase, raw); err == nil {
		return message, nil
	}
	return connectortools.GraviteeTicketOutcomeRenderer{}.RenderTicketOutcome(phase, raw)
}
