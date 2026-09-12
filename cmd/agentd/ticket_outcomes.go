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
		tickets, p.content, answers, connectortools.GraviteeTicketOutcomeRenderer{}, owner,
	)
	go channelSweepLoop(ctx, ticketOutcomeAnswer, channel.MetricTaskTicketOutcomes,
		"ticket outcomes delivered", metrics, func() (int, error) {
			return outcomes.Sweep(ctx, askLease, 20)
		})
}
