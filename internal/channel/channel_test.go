package channel_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
)

/*
What a run tells the people who are waiting on it.

The first stage of NT-005 and the one with no inbound surface at all: a run
reports to a conversation and nothing a conversation says can start anything.
That is deliberate — it is most of the value of the alert case, where the
trigger is the webhook and the channel is only where people find out.
*/

var noon = time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

func TestSweep_runIsWaitingOnSomebody_reportsToTheConversationsInItsScope(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	r := reporterFor(t, posts,
		report("run-1", "acme", "ops", channel.EventParked),
	)

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if len(posts.sent) != 1 {
		t.Fatalf("posted %d messages, want 1", len(posts.sent))
	}
	if posts.sent[0].conversation.ID != "C07-ops" {
		t.Errorf("posted to %s, want the conversation mapped to acme/ops", posts.sent[0].conversation.ID)
	}
}

func TestSweep_runIsInAnotherArea_reportsNowhere(t *testing.T) {
	t.Parallel()
	// The conversation carries the scope (NT-005 §4). A channel that received
	// another area's runs would be a way around every read check on the
	// platform, arriving as a notification.
	posts := &recorder{}
	r := reporterFor(t, posts,
		report("run-1", "acme", "finance", channel.EventParked),
	)

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(posts.sent) != 0 {
		t.Fatalf("posted %d messages about an area with no conversation", len(posts.sent))
	}
}

func TestSweep_conversationDoesNotWantThatEvent_staysQuiet(t *testing.T) {
	t.Parallel()
	// A channel that receives every run finishing is a channel people mute,
	// and a muted channel is worse than none: the approval that mattered is
	// in it.
	posts := &recorder{}
	r := reporterFor(t, posts,
		report("run-1", "acme", "ops", channel.EventFinished),
	)

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(posts.sent) != 0 {
		t.Fatalf("posted %d messages the conversation did not ask for", len(posts.sent))
	}
}

func TestSweep_alreadyReported_doesNotReportAgain(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	deliveries := &memoryDeliveries{}
	r := reporterFor(t, posts, report("run-1", "acme", "ops", channel.EventParked))
	r = r.WithDeliveries(deliveries)

	for range 3 {
		if _, err := r.Sweep(t.Context(), 50); err != nil {
			t.Fatalf("sweep: %v", err)
		}
	}
	if len(posts.sent) != 1 {
		t.Fatalf("posted %d times for one event", len(posts.sent))
	}
}

// Delivery is recorded after the message leaves, never before.
//
// The boundary offers no idempotency key, so exactly-once is not available at
// any price. Between two impossible guarantees this picks the one whose
// failure is noise: a duplicate approval request in a channel is an
// irritation, and an approval nobody ever saw is the thing this stage exists
// to prevent.
func TestSweep_postFails_leavesNothingRecordedSoTheNextPassRetries(t *testing.T) {
	t.Parallel()
	posts := &recorder{fail: channel.NewError(channel.CodeMissingScope, "slack is down")}
	deliveries := &memoryDeliveries{}
	r := reporterFor(t, posts, report("run-1", "acme", "ops", channel.EventParked)).
		WithDeliveries(deliveries)

	if _, err := r.Sweep(t.Context(), 50); err == nil {
		t.Fatal("a failed post was reported as a successful sweep")
	}
	if len(deliveries.recorded) != 0 {
		t.Fatalf("recorded %d deliveries for a message that never left", len(deliveries.recorded))
	}
	if len(deliveries.failures) != 1 {
		t.Fatalf("recorded %d failures, want 1", len(deliveries.failures))
	}
	failure := deliveries.failures[0]
	if failure.Code != channel.CodeMissingScope || failure.Scope.String() != "acme/ops" ||
		failure.Channel != "acme-slack" || failure.Conversation != "C07-ops" {
		t.Fatalf("failure = %+v, want scoped missing-scope for the refused conversation", failure)
	}

	posts.fail = nil
	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if posts.attempts != 2 || len(posts.sent) != 1 || len(deliveries.recorded) != 1 {
		t.Errorf("attempts=%d sent=%d deliveries=%d, want the second attempt to have"+
			" gone out and been recorded", posts.attempts, len(posts.sent), len(deliveries.recorded))
	}
}

func TestSweep_oneConversationRefuses_theOthersStillHear(t *testing.T) {
	t.Parallel()
	// Two areas, two conversations, one broken. A sweep that gave up on the
	// first failure would let one misconfigured channel silence every other.
	posts := &recorder{failFor: "C07-ops"}
	r := reporterFor(t, posts,
		report("run-1", "acme", "ops", channel.EventParked),
		report("run-2", "acme", "support", channel.EventParked),
	)

	if _, err := r.Sweep(t.Context(), 50); err == nil {
		t.Fatal("the sweep hid a conversation it could not reach")
	}
	if len(posts.sent) != 1 || posts.sent[0].conversation.ID != "C07-support" {
		t.Errorf("sent = %+v, want the reachable conversation to have heard", posts.sent)
	}
}

/*
A run one conversation could not hear is not a run that was reported.

The sweep finds candidates by asking which runs have not been said everywhere,
and a partial success that marked the run done would drop it from that
question for good: the conversation the bot was removed from is never tried
again, and the symptom is silence in one channel, which is the hardest failure
there is to notice.

The other direction has to hold too, or the fix trades one silence for a
repeat: a run that did reach everywhere must be marked, or every sweep for the
next day announces it again.
*/
func TestSweep_oneConversationRefuses_theRunIsNotMarkedReported(t *testing.T) {
	t.Parallel()
	posts := &recorder{failFor: "C07-ops"}
	reports := &fixedReports{reports: []channel.Report{
		report("run-1", "acme", "ops", channel.EventParked),
		report("run-2", "acme", "support", channel.EventParked),
	}}
	r := channel.NewReporter(reports, fixedConversations{}, posts,
		func() time.Time { return noon }, nil).WithDeliveries(&memoryDeliveries{})

	if _, err := r.Sweep(t.Context(), 50); err == nil {
		t.Fatal("the sweep hid a conversation it could not reach")
	}

	if slices.Contains(reports.done, domain.RunID("run-1")) {
		t.Error("a run one conversation never heard was marked as said everywhere")
	}
	if !slices.Contains(reports.done, domain.RunID("run-2")) {
		t.Error("a run every conversation heard was left to be announced again")
	}
}

/*
A run nobody could be told about is not a run that was reported.

The window exists so that turning a conversation on replays the last day into
it — that is what its comment says it is for. Marking a run said when there was
nowhere to say it spends that replay on silence: the conversation configured
five minutes later finds nothing waiting, and the run somebody is waiting on is
the one it loses.
*/
func TestSweep_noConversationWantsIt_leavesTheRunToBeAnnouncedLater(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	reports := &fixedReports{reports: []channel.Report{
		// An area with no conversation configured, which is the ordinary
		// shape of an installation part-way through being set up.
		report("run-1", "acme", "finance", channel.EventParked),
	}}
	r := channel.NewReporter(reports, fixedConversations{}, posts,
		func() time.Time { return noon }, nil).WithDeliveries(&memoryDeliveries{})

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if len(reports.done) != 0 {
		t.Errorf("marked as said where there was nobody to say it to: %v", reports.done)
	}
}

func TestNotice_gateRefusalRidesWithFailedNotifications(t *testing.T) {
	t.Parallel()

	posts := &recorder{}
	notice := channel.NewNotice(&fixedConversations{}, posts)

	n, err := notice.AnnounceCount(t.Context(),
		domain.Scope{Company: "acme", Area: "ops"},
		channel.Message{Event: channel.EventGateRefusal, Tool: "crm.delete_account"},
	)
	if err != nil {
		t.Fatalf("Announce: %v", err)
	}
	if n != 1 {
		t.Fatalf("AnnounceCount = %d, want one message sent", n)
	}
	if len(posts.sent) != 1 {
		t.Fatalf("sent %d messages, want one conversation that asked for failed events", len(posts.sent))
	}
	if posts.sent[0].conversation.ID != "C07-ops" {
		t.Errorf("conversation = %q, want #ops", posts.sent[0].conversation.ID)
	}

	posts = &recorder{}
	notice = channel.NewNotice(&fixedConversations{}, posts)
	n, err = notice.AnnounceCount(t.Context(),
		domain.Scope{Company: "acme", Area: "support"},
		channel.Message{Event: channel.EventGateRefusal, Tool: "crm.delete_account"},
	)
	if err != nil {
		t.Fatalf("Announce support: %v", err)
	}
	if n != 0 {
		t.Fatalf("AnnounceCount support = %d, want no message sent", n)
	}
	if len(posts.sent) != 0 {
		t.Fatalf("sent %d messages to a conversation that only asked for parked events", len(posts.sent))
	}
}

func report(run, company, area string, ev channel.Event) channel.Report {
	return channel.Report{
		RunID:   domain.RunID(run),
		AgentID: "triage",
		Scope:   domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)},
		Event:   ev,
		At:      noon,
	}
}

func reporterFor(t *testing.T, posts *recorder, reports ...channel.Report) *channel.Reporter {
	t.Helper()
	return channel.NewReporter(
		&fixedReports{reports: reports},
		&fixedConversations{},
		posts,
		func() time.Time { return noon },
		nil,
	).WithDeliveries(&memoryDeliveries{})
}

type fixedReports struct {
	reports []channel.Report
	// done records what the sweep declared said everywhere, which it must not
	// do for a run one conversation failed to hear.
	done []domain.RunID
}

func (f *fixedReports) Unreported(context.Context, time.Time, int) ([]channel.Report, error) {
	return f.reports, nil
}

func (f *fixedReports) Reported(
	_ context.Context, run domain.RunID, _ channel.Event, _ time.Time,
) error {
	f.done = append(f.done, run)
	return nil
}

// Two areas have a conversation and one does not, which is the ordinary shape
// of an installation part-way through being configured.
type fixedConversations struct{}

func (fixedConversations) For(_ context.Context, scope domain.Scope) ([]channel.Conversation, error) {
	switch scope.Area {
	case "ops":
		return []channel.Conversation{{
			Channel: "acme-slack", ID: "C07-ops", Label: "#ops",
			Wants: []channel.Event{channel.EventParked, channel.EventFailed},
		}}, nil
	case "support":
		return []channel.Conversation{{
			Channel: "acme-slack", ID: "C07-support", Label: "#suporte",
			Wants: []channel.Event{channel.EventParked},
		}}, nil
	}
	return nil, nil
}

type sent struct {
	conversation channel.Conversation
	message      channel.Message
}

type recorder struct {
	sent     []sent
	attempts int
	fail     error
	failFor  string
}

func (r *recorder) Post(_ context.Context, c channel.Conversation, m channel.Message) (string, error) {
	r.attempts++
	if r.fail != nil {
		return "", r.fail
	}
	if r.failFor != "" && c.ID == r.failFor {
		return "", errors.New("not in channel")
	}
	r.sent = append(r.sent, sent{conversation: c, message: m})
	return "1786000000.000100", nil
}

type memoryDeliveries struct {
	recorded []channel.Delivery
	failures []channel.DeliveryFailure
}

func (m *memoryDeliveries) Record(_ context.Context, d channel.Delivery) error {
	m.recorded = append(m.recorded, d)
	return nil
}

func (m *memoryDeliveries) RecordFailure(_ context.Context, f channel.DeliveryFailure) error {
	m.failures = append(m.failures, f)
	return nil
}

// The fake keys delivery the way the real store does. A fake that was more
// permissive about what counts as the same conversation would let a test pass
// against a namespacing the production table refuses.
func (m *memoryDeliveries) Delivered(
	_ context.Context, run domain.RunID, ev channel.Event, ch, conv string,
) (bool, error) {
	for _, d := range m.recorded {
		if d.RunID == run && d.Event == ev && d.Channel == ch && d.Conversation == conv {
			return true, nil
		}
	}
	return false, nil
}
