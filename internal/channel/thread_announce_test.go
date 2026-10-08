package channel_test

import (
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
)

/*
A run opened from a message belongs to that message's thread, and so does
everything the platform says about it: the approval card, the parked
notice. A card at the top level beside a conversation happening in a thread
is a question posted where nobody is looking.
*/
func TestSweep_anAskedRunsCard_followsTheOriginThread(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	parked := report("run-1", "acme", "ops", channel.EventParked)
	parked.AwaitingDecision = true
	parked.Asked = true
	parked.AskedChannel = "acme-slack"
	parked.AskedConversation = "C07-ops"
	parked.AskedThread = "171234.5678"

	r := channel.NewReporter(
		&fixedReports{reports: []channel.Report{parked}},
		rooms(roomWanting("C07-ops", channel.EventParked)),
		posts, func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{})

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(posts.sent) != 1 {
		t.Fatalf("sent = %+v, want one card", posts.sent)
	}
	if got := posts.sent[0].conversation.Thread; got != "171234.5678" {
		t.Errorf("Thread = %q, want the origin thread", got)
	}
}

// The thread travels only to the conversation the ask came from: the same
// run announced to a second configured room posts at that room's top level,
// because the thread id belongs to one channel's message, not to the run.
func TestSweep_theOriginThread_staysInItsOwnConversation(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	parked := report("run-1", "acme", "ops", channel.EventParked)
	parked.Asked = true
	parked.AskedChannel = "acme-slack"
	parked.AskedConversation = "C07-ops"
	parked.AskedThread = "171234.5678"

	r := channel.NewReporter(
		&fixedReports{reports: []channel.Report{parked}},
		rooms(
			roomWanting("C07-ops", channel.EventParked),
			roomWanting("C99-audit", channel.EventParked),
		),
		posts, func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{})

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	threads := map[string]string{}
	for _, one := range posts.sent {
		threads[one.conversation.ID] = one.conversation.Thread
	}
	if threads["C07-ops"] != "171234.5678" || threads["C99-audit"] != "" {
		t.Fatalf("threads = %+v", threads)
	}
}

// A run nobody asked for is byte-identical to before: top level everywhere.
func TestSweep_anUnaskedRun_staysAtTheTopLevel(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	parked := report("run-1", "acme", "ops", channel.EventParked)

	r := channel.NewReporter(
		&fixedReports{reports: []channel.Report{parked}},
		rooms(roomWanting("C07-ops", channel.EventParked)),
		posts, func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{})

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(posts.sent) != 1 || posts.sent[0].conversation.Thread != "" {
		t.Fatalf("sent = %+v, want one top-level card", posts.sent)
	}
}
