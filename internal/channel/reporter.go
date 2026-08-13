package channel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
Reporter says what has happened, once.

A sweep rather than a hook on the run that parked, for the reason the event
dispatcher is one too: a worker that died between parking a run and announcing
it would drop the announcement, and a sweep that runs again cannot.

Delivery is recorded after the message leaves and never before. The boundary
offers no idempotency key, so exactly-once is not for sale at any price;
between the two failures available this takes the one that is noise. A repeated
approval request in a channel is an irritation. An approval nobody ever saw is
the thing this stage exists to prevent.
*/
type Reporter struct {
	reports       Reports
	conversations Conversations
	poster        Poster
	deliveries    Deliveries
	clock         func() time.Time
	baseURL       string
	log           *slog.Logger
}

// Window is how far back a first sweep looks.
//
// Bounded so that configuring a conversation does not replay a year of runs
// into it, and generous enough that a process away for the afternoon still
// says what it missed.
const Window = 24 * time.Hour

func NewReporter(
	reports Reports, conversations Conversations, poster Poster,
	clock func() time.Time, log *slog.Logger,
) *Reporter {
	if log == nil {
		log = slog.Default()
	}
	return &Reporter{
		reports: reports, conversations: conversations,
		poster: poster, clock: clock, log: log,
		deliveries: noDeliveries{},
	}
}

// WithDeliveries records what has been said. Without it nothing is remembered
// and every sweep repeats itself, which is why it is not optional in practice.
func (r *Reporter) WithDeliveries(d Deliveries) *Reporter {
	r.deliveries = d
	return r
}

// WithBaseURL is where a reader goes to act on what they were told. A
// notification about an approval that does not link to the approval is a
// notification that makes somebody go looking.
func (r *Reporter) WithBaseURL(base string) *Reporter {
	r.baseURL = base
	return r
}

// Sweep says what has not been said, and answers how many messages left.
//
// One unreachable conversation does not silence the others: a workspace the
// bot was removed from would otherwise stop every area's notifications, and
// the failure would show up as silence, which is the hardest thing to notice.
func (r *Reporter) Sweep(ctx context.Context, limit int) (int, error) {
	pending, err := r.reports.Unreported(ctx, r.clock().Add(-Window), limit)
	if err != nil {
		return 0, fmt.Errorf("channel: read what is unreported: %w", err)
	}

	sent, failures := 0, []error{}
	for _, report := range pending {
		n, err := r.announce(ctx, report)
		sent += n
		if err != nil {
			failures = append(failures, err)
		}
	}
	return sent, errors.Join(failures...)
}

// announce tells every conversation that speaks for the run's scope and wants
// to hear about this.
func (r *Reporter) announce(ctx context.Context, report Report) (int, error) {
	places, err := r.conversations.For(ctx, report.Scope)
	if err != nil {
		return 0, fmt.Errorf("channel: conversations for %s: %w", report.Scope, err)
	}

	sent, failures := 0, []error{}
	for _, place := range places {
		if !place.wants(report.Event) {
			continue
		}
		posted, err := r.post(ctx, report, place)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if posted {
			sent++
		}
	}
	return sent, errors.Join(failures...)
}

// post sends one message, unless it has already been sent.
func (r *Reporter) post(ctx context.Context, report Report, place Conversation) (bool, error) {
	said, err := r.deliveries.Delivered(ctx, report.RunID, report.Event, place.ID)
	if err != nil {
		return false, fmt.Errorf("channel: read deliveries: %w", err)
	}
	if said {
		return false, nil
	}

	ref, err := r.poster.Post(ctx, place, r.message(report))
	if err != nil {
		// Named, because the ordinary cause is a bot removed from one channel
		// and the symptom is silence in that channel alone.
		return false, fmt.Errorf("channel: post to %s: %w", place.Label, err)
	}

	return true, r.deliveries.Record(ctx, Delivery{
		RunID: report.RunID, Event: report.Event, Conversation: place.ID,
		Ref: ref, PostedAt: r.clock(),
	})
}

func (r *Reporter) message(report Report) Message {
	m := Message{
		Event: report.Event, RunID: report.RunID, Agent: report.AgentID,
		Scope: report.Scope, Reason: report.Reason, Tool: report.Tool,
	}
	if r.baseURL != "" {
		m.Link = fmt.Sprintf("%s/runs/%s", r.baseURL, report.RunID)
	}
	return m
}

// noDeliveries remembers nothing, which makes every sweep repeat itself. It is
// the zero value rather than a nil check so a Reporter built without a store
// fails loudly in a channel instead of panicking in a worker.
type noDeliveries struct{}

func (noDeliveries) Record(context.Context, Delivery) error { return nil }
func (noDeliveries) Delivered(context.Context, domain.RunID, Event, string) (bool, error) {
	return false, nil
}
