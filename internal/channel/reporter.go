package channel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
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
	approvers     Approvers
	accounts      Accounts
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

/*
WithDirectApprovals lets a conversation also tell the people who may decide.

Optional because most of this platform's outbound path has nothing to do with
approvals, and a reporter without it simply never sends a private message —
which is what an installation that has not opted in gets anyway.
*/
func (r *Reporter) WithDirectApprovals(who Approvers, where Accounts) *Reporter {
	r.approvers, r.accounts = who, where
	return r
}

// WithConversations replaces where announcements go. Used by tests that need a
// shape the default map does not describe.
func (r *Reporter) WithConversations(c Conversations) *Reporter {
	r.conversations = c
	return r
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

	// One fan-out for the pass. Who may decide and where they are reachable
	// are asked once and remembered: both are configuration, and re-reading
	// them between reports would let the set of people one sweep messages
	// change halfway through the sweep.
	pass := r.newFanout()
	sent, failures := 0, []error{}
	for _, report := range pending {
		n, told, err := r.announce(ctx, pass, report)
		sent += n
		if err != nil {
			// Left unreported on purpose. The next sweep tries the
			// conversations that did not hear, and the ones that did are
			// skipped at the post rather than told twice.
			failures = append(failures, err)
			continue
		}
		if told == 0 {
			// Nowhere to say it. Marking it said would spend the window on
			// silence: the whole reason the window exists is that turning a
			// conversation on replays the last day into it, and a run marked
			// here is a run that conversation never hears about.
			continue
		}
		if err := r.reports.Reported(ctx, report, r.clock()); err != nil {
			failures = append(failures, err)
		}
	}
	return sent, errors.Join(failures...)
}

// announce tells every conversation that speaks for the run's scope and wants
// to hear about this.
//
// It answers how many messages left and how many conversations were owed one
// at all — which are different questions. Nothing sent because everybody had
// already heard is finished; nothing sent because nobody was listening is not.
func (r *Reporter) announce(
	ctx context.Context, pass *fanout, report Report,
) (sent, told int, err error) {
	places, err := r.conversations.For(ctx, report.Scope)
	if err != nil {
		err = WrapError(
			CodeConfigurationReadFailed,
			fmt.Errorf("channel: conversations for %s: %w", report.Scope, err),
		)
		return 0, 0, errors.Join(err, r.recordFailures(ctx, r.failuresFor(report, Conversation{}, err)))
	}

	failures := []error{}
	deliveryFailures := []DeliveryFailure{}
	for _, place := range places {
		if !place.wants(report.Event) || !place.reportsAgent(report.AgentID) {
			continue
		}
		// Owed a message, whether or not one goes out now: a conversation
		// that already heard is one this run has finished with.
		told++

		n, refused := r.tell(ctx, pass, report, place)
		sent += n
		failures = append(failures, refused.blocking...)
		deliveryFailures = append(deliveryFailures, refused.recorded...)
	}
	if err := r.recordFailures(ctx, deliveryFailures); err != nil {
		failures = append(failures, err)
	}
	return sent, told, errors.Join(failures...)
}

func (r *Reporter) failuresFor(report Report, place Conversation, cause error) []DeliveryFailure {
	var failures []DeliveryFailure
	for _, code := range FailureCodes(cause) {
		failures = append(failures, DeliveryFailure{
			Announcement: report.AnnouncementTo(place),
			ScopeWide:    place.Channel == "" && place.ID == "",
			Code:         code, Scope: report.Scope, AgentID: report.AgentID,
			SeenAt: r.clock(),
		})
	}
	return failures
}

func (r *Reporter) recordFailures(ctx context.Context, failures []DeliveryFailure) error {
	if len(failures) == 0 {
		return nil
	}
	if err := r.deliveries.RecordFailures(ctx, failures); err != nil {
		return fmt.Errorf("channel: record delivery failures: %w", err)
	}
	return nil
}

// post sends one message, unless it has already been sent.
func (r *Reporter) post(ctx context.Context, report Report, place Conversation) (bool, error) {
	owed := report.AnnouncementTo(place)
	said, err := r.deliveries.Delivered(ctx, owed)
	if err != nil {
		return false, fmt.Errorf("channel: read deliveries: %w", err)
	}
	if said {
		return false, nil
	}

	at, err := placed(ctx, r.poster, place, r.message(report))
	if err != nil {
		// Named, because the ordinary cause is a bot removed from one channel
		// and the symptom is silence in that channel alone.
		return false, fmt.Errorf("channel: post to %s: %w", place.Label, err)
	}

	return true, r.deliveries.Record(ctx, Delivery{
		Announcement: owed, Ref: at.Ref, Placed: at.Conversation, PostedAt: r.clock(),
	})
}

func (r *Reporter) message(report Report) Message {
	m := Message{
		Event: report.Event, RunID: report.RunID, Agent: report.AgentID,
		Scope: report.Scope, Reason: report.Reason, Tool: report.Tool,
		AtSeq: report.AtSeq,
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
func (noDeliveries) RecordFailure(context.Context, DeliveryFailure) error {
	return nil
}
func (noDeliveries) RecordFailures(context.Context, []DeliveryFailure) error {
	return nil
}
func (noDeliveries) Delivered(context.Context, Announcement) (bool, error) {
	return false, nil
}
