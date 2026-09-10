package channel_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
)

/*
Telling the people who may decide, as well as the room.

The card in a channel is addressed to whoever happens to be looking. Somebody
who can release the run in seconds has to notice it among everything else, and
in a busy conversation that is how a run sits parked for hours. So the same
card also goes privately to the people the platform knows may decide it.

**It addresses and it does not authorise.** The button carries the run and the
step, and pressing it takes the console's path with the console's checks
against the run's own scope. Being sent one and being allowed to answer it are
different facts, and the second is never decided here.

**And it never replaces the room.** The ledger records every decision, but the
ambient visibility that makes somebody notice a run has been waiting is not in
the ledger, it is in the channel — so the channel card goes first, and a
conversation that could not be told is not quietly answered by a private
message instead.
*/

func TestSweep_aParkedRunInAnOptedInConversation_alsoReachesTheDecidersPrivately(t *testing.T) {
	posts := &recorder{}
	r := directReporter(t, posts, deciders("usr_ana", "usr_bruno"),
		accountBook{"acme-slack": {"usr_ana": "U-ana", "usr_bruno": "U-bruno"}},
		parkedReport())

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	// The room first, then the people. Order matters: a private card that
	// arrived instead of the channel card would have replaced the only place
	// anybody else can see the run is waiting.
	if len(posts.sent) != 3 {
		t.Fatalf("sent %d messages, want the room and two people", len(posts.sent))
	}
	if posts.sent[0].conversation.ID != "C07-ops" {
		t.Errorf("first went to %q, want the room", posts.sent[0].conversation.ID)
	}
	for _, want := range []string{"U-ana", "U-bruno"} {
		if !addressed(posts.sent, want) {
			t.Errorf("nothing went to %q", want)
		}
	}
}

// A conversation that did not ask for this is unchanged. The option is the
// whole of the decision; nothing infers it from the presence of approvers.
func TestSweep_aConversationThatDidNotAskForIt_tellsOnlyTheRoom(t *testing.T) {
	posts := &recorder{}
	r := directReporter(t, posts, deciders("usr_ana"),
		accountBook{"acme-slack": {"usr_ana": "U-ana"}},
		parkedReport())
	r = r.WithConversations(rooms(room("C07-ops", false)))

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(posts.sent) != 1 || posts.sent[0].conversation.ID != "C07-ops" {
		t.Fatalf("sent = %+v, want the room alone", posts.sent)
	}
}

/*
Only a stopped run, and only one with a step to answer.

The condition is the one that draws the buttons. A private message about a run
that finished carries no decision and is a notification nobody asked for; one
about a park with no step names nothing the button could answer.
*/
func TestSweep_anythingButAParkedRunWithAStep_reachesNobodyPrivately(t *testing.T) {
	for _, c := range []struct {
		name   string
		report channel.Report
	}{
		{"finished", reportOf(channel.EventFinished, 4)},
		{"failed", reportOf(channel.EventFailed, 4)},
		{"parked with no step", reportOf(channel.EventParked, 0)},
	} {
		posts := &recorder{}
		r := directReporter(t, posts, deciders("usr_ana"),
			accountBook{"acme-slack": {"usr_ana": "U-ana"}}, c.report)
		r = r.WithConversations(rooms(room("C07-ops", true,
			channel.EventParked, channel.EventFailed, channel.EventFinished)))

		if _, err := r.Sweep(context.Background(), 10); err != nil {
			t.Fatalf("%s: Sweep: %v", c.name, err)
		}
		if addressed(posts.sent, "U-ana") {
			t.Errorf("%s: reached somebody privately", c.name)
		}
	}
}

// Somebody nobody bound is skipped, and skipped is not a delivery. An empty
// conversation is the shape that means a run was said everywhere, and the
// store refuses to record one — this must never reach it.
func TestSweep_adeciderWithNoBoundAccount_recordsNothingAndDoesNotFail(t *testing.T) {
	posts := &recorder{}
	deliveries := &memoryDeliveries{}
	r := directReporter(t, posts, deciders("usr_ana", "usr_unbound"),
		accountBook{"acme-slack": {"usr_ana": "U-ana"}},
		parkedReport()).WithDeliveries(deliveries)

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	for _, d := range deliveries.recorded {
		if d.Conversation == "" {
			t.Fatal("recorded a delivery naming no conversation")
		}
	}
	if !addressed(posts.sent, "U-ana") {
		t.Error("the person who is bound was not told")
	}
}

// The same person is told once per question, however many sweeps run. The
// account is the conversation, so the dedup that already keeps a room from
// hearing twice keeps a person from being messaged twice.
func TestSweep_runTwice_tellsEachPersonOnce(t *testing.T) {
	posts := &recorder{}
	deliveries := &memoryDeliveries{}
	r := directReporter(t, posts, deciders("usr_ana"),
		accountBook{"acme-slack": {"usr_ana": "U-ana"}},
		parkedReport()).WithDeliveries(deliveries)

	for range 3 {
		if _, err := r.Sweep(context.Background(), 10); err != nil {
			t.Fatalf("Sweep: %v", err)
		}
	}
	if n := countAddressed(posts.sent, "U-ana"); n != 1 {
		t.Errorf("told %d times, want once", n)
	}
}

/*
Past the cap, nobody is told privately and the reason is recorded.

An approver grant held at the installation scope covers every run in it, so
"the people who may decide" can be a hundred. Messaging an arbitrary twenty of
them is worse than messaging none: nobody can tell whether they were meant to
be asked, and the nineteen who were not have no way to know they were skipped.
The room still hears, which is the point of the room.
*/
func TestSweep_moreDecidersThanTheCap_tellsTheRoomAndNobodyPrivately(t *testing.T) {
	who := make([]domain.UserID, 0, channel.MaxDirectRecipients+1)
	book := accountBook{"acme-slack": {}}
	for i := range channel.MaxDirectRecipients + 1 {
		id := domain.UserID(fmt.Sprintf("usr_%02d", i))
		who = append(who, id)
		book["acme-slack"][id] = fmt.Sprintf("U-%02d", i)
	}

	posts := &recorder{}
	deliveries := &memoryDeliveries{}
	r := directReporter(t, posts, &fixedApprovers{who: who}, book,
		parkedReport()).WithDeliveries(deliveries)

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(posts.sent) != 1 || posts.sent[0].conversation.ID != "C07-ops" {
		t.Fatalf("sent %d messages, want the room alone", len(posts.sent))
	}
	if !recordedFailure(deliveries, channel.CodeTooManyRecipients) {
		t.Errorf("failures = %+v, want the cap recorded", deliveries.failures)
	}
}

/*
A refusal nothing can fix leaves the room told and the run finished with.

An installation that never granted the app permission to open a direct message
would otherwise retry every recipient every thirty seconds for a day, and the
run would never be marked as announced — so the channel card, which did arrive,
would be re-sent to a conversation that already heard it as soon as anything
else changed.
*/
func TestSweep_aPermanentDirectFailure_stillMarksTheRunReported(t *testing.T) {
	posts := &recorder{failFor: "U-ana", failWith: channel.NewError(
		channel.CodeMissingScope, "slack: refused: missing_scope")}
	deliveries := &memoryDeliveries{}
	reports := &fixedReports{reports: []channel.Report{parkedReport()}}
	r := directReporterWith(t, reports, posts, deciders("usr_ana"),
		accountBook{"acme-slack": {"usr_ana": "U-ana"}}).WithDeliveries(deliveries)

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(reports.done) != 1 {
		t.Errorf("done = %v, want the run marked reported", reports.done)
	}
	if !recordedFailure(deliveries, channel.CodeMissingScope) {
		t.Errorf("failures = %+v, want the refusal recorded", deliveries.failures)
	}
}

// A refusal another sweep could survive keeps the run open, and the next pass
// skips the room and the people already reached.
func TestSweep_aTransientDirectFailure_leavesTheRunForTheNextPass(t *testing.T) {
	posts := &recorder{failFor: "U-bruno", failWith: channel.NewError(
		channel.CodeRateLimited, "slack: rate limited")}
	deliveries := &memoryDeliveries{}
	reports := &fixedReports{reports: []channel.Report{parkedReport()}}
	r := directReporterWith(t, reports, posts, deciders("usr_ana", "usr_bruno"),
		accountBook{"acme-slack": {"usr_ana": "U-ana", "usr_bruno": "U-bruno"}}).
		WithDeliveries(deliveries)

	if _, err := r.Sweep(context.Background(), 10); err == nil {
		t.Fatal("Sweep reported success with a recipient left untold")
	}
	if len(reports.done) != 0 {
		t.Errorf("done = %v, want the run left for the next pass", reports.done)
	}

	posts.failFor, posts.failWith = "", nil
	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("second Sweep: %v", err)
	}
	if n := countAddressed(posts.sent, "U-ana"); n != 1 {
		t.Errorf("told the person who already heard %d times, want once", n)
	}
	if !addressed(posts.sent, "U-bruno") {
		t.Error("the person who was rate limited was never told")
	}
}

// A room that could not be told is not answered with a private message
// instead. The private card is in addition to the collective one, never
// standing in for it.
func TestSweep_theRoomCouldNotBeTold_doesNotTellAnybodyPrivately(t *testing.T) {
	posts := &recorder{failFor: "C07-ops", failWith: errors.New("not in channel")}
	r := directReporter(t, posts, deciders("usr_ana"),
		accountBook{"acme-slack": {"usr_ana": "U-ana"}}, parkedReport())

	if _, err := r.Sweep(context.Background(), 10); err == nil {
		t.Fatal("Sweep reported success with the room untold")
	}
	if addressed(posts.sent, "U-ana") {
		t.Error("a private message went out in place of the channel card")
	}
}

// Asked once. The people who may decide are a property of the run's scope and
// the bindings are configuration, so re-reading either between recipients
// would let the set of people a message reaches change halfway through.
func TestSweep_twoReportsInOneScope_asksWhoAndWhereOnce(t *testing.T) {
	posts := &recorder{}
	who := deciders("usr_ana")
	book := &countingAccounts{book: accountBook{"acme-slack": {"usr_ana": "U-ana"}}}
	r := directReporterWith(t,
		&fixedReports{reports: []channel.Report{parkedReport(), otherParkedReport()}},
		posts, who, book)

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if who.calls != 1 {
		t.Errorf("asked who may approve %d times, want once for the scope", who.calls)
	}
	if book.calls != 1 {
		t.Errorf("read the bindings %d times, want once for the connection", book.calls)
	}
}

// --- the harness ------------------------------------------------------------

func parkedReport() channel.Report { return reportOf(channel.EventParked, 4) }
func otherParkedReport() channel.Report {
	r := reportOf(channel.EventParked, 7)
	r.RunID = "run-second"
	return r
}

func reportOf(event channel.Event, atSeq int64) channel.Report {
	return channel.Report{
		RunID: "run-1", AgentID: "triage", Event: event, AtSeq: atSeq,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
		Tool:  "erp.transfer", Reason: "over the ceiling", At: noon,
		// A stop is a question by default here, because that is what these
		// tests are about. The one that is not says so.
		AwaitingDecision: event == channel.EventParked,
	}
}

func room(id string, direct bool, wants ...channel.Event) channel.Conversation {
	if len(wants) == 0 {
		wants = []channel.Event{channel.EventParked}
	}
	return channel.Conversation{
		Channel: "acme-slack", ID: id, Label: "#" + id,
		Wants: wants, DirectApprovals: direct,
	}
}

func rooms(places ...channel.Conversation) channel.Conversations {
	return multiConversations{places: places}
}

func directReporter(
	t *testing.T, posts *recorder, who channel.Approvers,
	where channel.Accounts, reports ...channel.Report,
) *channel.Reporter {
	t.Helper()
	return directReporterWith(t, &fixedReports{reports: reports}, posts, who, where)
}

func directReporterWith(
	t *testing.T, reports channel.Reports, posts *recorder,
	who channel.Approvers, where channel.Accounts,
) *channel.Reporter {
	t.Helper()
	return channel.NewReporter(
		reports, rooms(room("C07-ops", true)), posts,
		func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{}).WithDirectApprovals(who, where)
}

type fixedApprovers struct {
	who   []domain.UserID
	calls int
}

func (f *fixedApprovers) ApproversIn(context.Context, domain.Scope) ([]domain.UserID, error) {
	f.calls++
	return f.who, nil
}

func deciders(ids ...string) *fixedApprovers {
	who := make([]domain.UserID, 0, len(ids))
	for _, id := range ids {
		who = append(who, domain.UserID(id))
	}
	return &fixedApprovers{who: who}
}

// accountBook is the bindings, by connection. Absence is the ordinary answer:
// most people in a workspace have never been bound.
type accountBook map[string]map[domain.UserID]string

func (b accountBook) AccountsOn(
	_ context.Context, channelName string, who []domain.UserID,
) (map[domain.UserID]string, error) {
	out := map[domain.UserID]string{}
	for _, one := range who {
		if account, bound := b[channelName][one]; bound {
			out[one] = account
		}
	}
	return out, nil
}

type countingAccounts struct {
	book  accountBook
	calls int
}

func (c *countingAccounts) AccountsOn(
	ctx context.Context, channelName string, who []domain.UserID,
) (map[domain.UserID]string, error) {
	c.calls++
	return c.book.AccountsOn(ctx, channelName, who)
}

func addressed(sent []sent, id string) bool { return countAddressed(sent, id) > 0 }

func countAddressed(sent []sent, id string) int {
	n := 0
	for _, one := range sent {
		if one.conversation.ID == id {
			n++
		}
	}
	return n
}

func recordedFailure(d *memoryDeliveries, code string) bool {
	for _, f := range d.failures {
		if strings.Contains(f.Code, channel.MetricCode(code)) {
			return true
		}
	}
	return false
}

/*
A run that stopped without asking anybody is not an approval.

A budget park and a retry that stopped helping carry a sequence now — the
announcement is keyed by the step a run stopped on, and for those the step is
the run's own last one. Read as "a park with a sequence is a pending
approval", every one of them messages the approvers privately and draws two
buttons whose only possible answer is a conflict.

Whether a decision is pending is a phase and not a number.
*/
func TestSweep_aParkWithNothingToDecide_reachesNobodyPrivately(t *testing.T) {
	posts := &recorder{}
	stopped := reportOf(channel.EventParked, 6)
	stopped.AwaitingDecision = false
	r := directReporter(t, posts, deciders("usr_ana"),
		accountBook{"acme-slack": {"usr_ana": "U-ana"}}, stopped)

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if addressed(posts.sent, "U-ana") {
		t.Error("a stop with nothing to decide was sent to an approver privately")
	}
	if posts.sent[0].message.AwaitingDecision {
		t.Error("the channel card offers buttons for a stop with nothing to decide")
	}
}

/*
A failure another sweep could survive never retires the announcement.

Only a refusal that means the same thing next time may degrade to the channel
alone: an app that was never granted permission to open a direct message, a
person who cannot be messaged, a driver that cannot do it at all. A reset
connection, an unreadable answer, a directory that was away — those are this
side being unavailable, and treating them as final loses the message for good
while marking the run as told.
*/
func TestSweep_aDirectFailureThatMightPass_leavesTheRunForTheNextPass(t *testing.T) {
	for _, cause := range []struct {
		name string
		err  error
	}{
		{"a reset connection", channel.WrapError(
			channel.CodeDeliveryFailed, errors.New("connection reset by peer"))},
		{"an answer nobody could read", channel.NewError(
			channel.CodeDeliveryFailed, "slack: read answer (status 502)")},
		{"a refusal with no code at all", errors.New("something went wrong")},
	} {
		posts := &recorder{failFor: "U-ana", failWith: cause.err}
		reports := &fixedReports{reports: []channel.Report{parkedReport()}}
		r := directReporterWith(t, reports, posts, deciders("usr_ana"),
			accountBook{"acme-slack": {"usr_ana": "U-ana"}})

		if _, err := r.Sweep(context.Background(), 10); err == nil {
			t.Errorf("%s: Sweep reported success with the message lost", cause.name)
		}
		if len(reports.done) != 0 {
			t.Errorf("%s: the run was marked reported", cause.name)
		}
	}
}

// And a directory that was away is the same kind of absence. Nobody was told
// and nothing about that is settled.
func TestSweep_theApproversCouldNotBeRead_leavesTheRunForTheNextPass(t *testing.T) {
	posts := &recorder{}
	reports := &fixedReports{reports: []channel.Report{parkedReport()}}
	r := directReporterWith(t, reports, posts, &failingApprovers{},
		accountBook{"acme-slack": {"usr_ana": "U-ana"}})

	if _, err := r.Sweep(context.Background(), 10); err == nil {
		t.Fatal("Sweep reported success without knowing who to tell")
	}
	if len(reports.done) != 0 {
		t.Errorf("done = %v, want the run left for the next pass", reports.done)
	}
}

/*
The cap counts messages, not candidates.

Twenty-one people may decide and one of them is on Slack. Counted before the
bindings are read, that is a scope past the cap and nobody is told — so
somebody unbound silences the one person who could have been reached, which is
the opposite of what the cap is for.
*/
func TestSweep_moreApproversThanTheCapButFewReachable_tellsThemAnyway(t *testing.T) {
	who := make([]domain.UserID, 0, channel.MaxDirectRecipients+1)
	for i := range channel.MaxDirectRecipients + 1 {
		who = append(who, domain.UserID(fmt.Sprintf("usr_%02d", i)))
	}

	posts := &recorder{}
	r := directReporter(t, posts, &fixedApprovers{who: who},
		accountBook{"acme-slack": {"usr_00": "U-00"}}, parkedReport())

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if !addressed(posts.sent, "U-00") {
		t.Error("the one reachable person was silenced by twenty who are not")
	}
}

// failingApprovers is the directory being away, and counts how often it was
// asked: a pass that re-asks during an outage amplifies it once per report.
type failingApprovers struct{ calls int }

func (f *failingApprovers) ApproversIn(context.Context, domain.Scope) ([]domain.UserID, error) {
	f.calls++
	return nil, errors.New("the directory is away")
}
