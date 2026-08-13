package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/connect"
	"github.com/fuseone/agents/internal/settings"
)

// channelSweep is how often runs report to their conversations.
//
// Short, because the thing it announces is a run waiting on a person and every
// minute of that is a minute somebody is not being asked. Cheap enough to be
// short: the query answers from an index and finds nothing on most passes.
const channelSweep = 30 * time.Second

// reportToChannels tells the conversations configured for a scope what its runs
// are doing.
//
// Outbound only, and that is the whole of stage one (NT-005 §7). Nothing a
// conversation says reaches this process; the message even links to the console
// rather than offering a button, because a button would promise an inbound
// surface that does not exist yet.
func reportToChannels(ctx context.Context, store *settings.Store, deliveries *channel.Postgres, baseURL string) {
	reporter := channel.NewReporter(
		deliveries,
		channel.NewConfigured(store),
		channel.NewRouter(connect.New(store)),
		time.Now,
		slog.Default(),
	).WithDeliveries(deliveries).WithBaseURL(baseURL)

	ticker := time.NewTicker(channelSweep)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sent, err := reporter.Sweep(ctx, 50)
			if err != nil {
				// Logged and continued. One conversation the bot was removed
				// from must not stop the sweep that still reaches the others,
				// and the symptom of that failure is silence — which is the
				// hardest thing to notice.
				slog.Error("some runs could not be announced", "err", err)
			}
			if sent > 0 {
				slog.Info("runs announced to channels", "messages", sent)
			}
		}
	}
}
