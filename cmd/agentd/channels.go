package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/admin"
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

/*
The two halves of an inbound ask, each on its own clock.

Opening and answering are separate sweeps because they fail differently. A
conversation the bot was removed from must not hold up the runs other people
are waiting on, and a slow vendor answering fifty refusals must not delay the
next ask by the length of its own timeout.

They share one consumer, which holds nothing that changes after it is wired,
and one owner name — the claims are told apart by the state they take, never by
who took them.
*/

// askSweep is how often an ask that arrived becomes a run. Short: somebody
// typed a question and is watching the conversation for an answer.
const askSweep = 5 * time.Second

// askAnswer is how often refusals nobody has been told about go out. Slower
// than opening, because it is the second-best outcome by definition and every
// pass of it costs a call to somebody else's API.
const askAnswer = 10 * time.Second

/*
askLease is how long one consumer holds an ask before another may take it.

Longer than opening takes and far shorter than a person's patience. Too short
and two workers open the same ask — which the idempotency key survives, and
which still costs two of everything. Too long and an ask claimed by a process
that died waits out the lease before anybody else may try.
*/
const askLease = time.Minute

// consumeAsks turns what arrived in a conversation into runs, and says why
// when it will not (NT-005 stage 3).
func (p *workerParts) consumeAsks(ctx context.Context, owner string) {
	pool := p.configPool
	consumer := channel.NewConsumer(channel.NewInbox(pool), owner, slog.Default()).
		With(
			channel.NewConfigured(p.settings), // which scope a conversation speaks for
			p.registry,                        // what could be started there
			p.registry,                        // and which of those agreed to be
			channel.NewPostgres(pool),         // what a thread is about
			channel.FromTrigger(p.opener()),   // the same pauses and stops as a schedule
			connect.New(p.settings),           // and the bot that says it back
		).
		Binding(admin.NewChannels(pool, p.settings).PrincipalFor)

	go sweep(ctx, askSweep, "asks opened", func() (int, error) {
		return consumer.Sweep(ctx, askLease, 20)
	})
	go sweep(ctx, askAnswer, "refusals delivered", func() (int, error) {
		return consumer.Answer(ctx, askLease, 20)
	})
}

/*
sweep runs one of them until the worker stops.

Failures are logged and the loop continues, for the reason the reporter's does:
one unreachable conversation must not silence the rest, and the symptom of that
failure would be silence — the hardest thing to notice.
*/
func sweep(ctx context.Context, every time.Duration, did string, pass func() (int, error)) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := pass()
			if err != nil {
				slog.Error("a channel sweep did not finish", "doing", did, "err", err)
			}
			if n > 0 {
				slog.Info(did, "count", n)
			}
		}
	}
}
