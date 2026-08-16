package channel_test

import (
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
)

/*
An ask is written down before it is acknowledged.

A channel wants an answer in seconds and retries what it does not get, so the
run cannot open on the request. Moving the work off the request solves the
retry and not the crash: between acknowledging and opening there is a window,
and a process that dies inside it has told the sender the ask arrived and holds
no record that it did.
*/

func TestReceive_anAskArrives_andIsWaitingToBeOpened(t *testing.T) {
	inbox := freshInbox(t)
	arrival := ask("ev-1")

	fresh, err := inbox.Receive(t.Context(), arrival)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if !fresh {
		t.Error("the first delivery of an ask reported as already seen")
	}

	waiting, err := inbox.Claim(t.Context(), "worker-1", time.Minute, 10)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(waiting) != 1 || waiting[0].EventID != "ev-1" {
		t.Errorf("claimed = %+v, want the ask", waiting)
	}
}

/*
A redelivery is not a second ask.

Every sender in existence redelivers, and a channel that opened a second run
for the same message would be one nobody could use twice. The conflict is the
mechanism: the caller acknowledges and queues nothing.
*/
func TestReceive_theSameDeliveryTwice_isNotASecondAsk(t *testing.T) {
	inbox := freshInbox(t)
	arrival := ask("ev-2")

	if _, err := inbox.Receive(t.Context(), arrival); err != nil {
		t.Fatalf("first Receive: %v", err)
	}
	fresh, err := inbox.Receive(t.Context(), arrival)
	if err != nil {
		t.Fatalf("second Receive: %v", err)
	}
	if fresh {
		t.Error("a redelivery reported as a new ask")
	}

	waiting, _ := inbox.Claim(t.Context(), "worker-1", time.Minute, 10)
	if len(waiting) != 1 {
		t.Errorf("claimed = %d asks, want the one", len(waiting))
	}
}

func TestOpened_anAskThatBecameARun_stopsWaiting(t *testing.T) {
	inbox := freshInbox(t)
	arrival := ask("ev-3")
	if _, err := inbox.Receive(t.Context(), arrival); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	held, err := inbox.Claim(t.Context(), "worker-1", time.Minute, 10)
	if err != nil || len(held) != 1 {
		t.Fatalf("Claim: %v (%d)", err, len(held))
	}
	if err := inbox.Opened(t.Context(), held[0], "run_1", time.Now()); err != nil {
		t.Fatalf("Opened: %v", err)
	}

	waiting, _ := inbox.Claim(t.Context(), "worker-2", time.Minute, 10)
	if len(waiting) != 0 {
		t.Errorf("claimed = %+v, want nothing left", waiting)
	}
}

/*
An ask that became nothing is kept, and says why.

"Somebody mentioned an agent that cannot be started here" is what an operator
needs when the person says nothing happened. Deleted on refusal, that
conversation is unanswerable: the ask is gone and the platform has no memory of
ever having been asked.
*/
func TestRefused_anAskThatBecameNothing_isKeptWithItsReason(t *testing.T) {
	inbox := freshInbox(t)
	arrival := ask("ev-4")
	if _, err := inbox.Receive(t.Context(), arrival); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	held, err := inbox.Claim(t.Context(), "worker-1", time.Minute, 10)
	if err != nil || len(held) != 1 {
		t.Fatalf("Claim: %v (%d)", err, len(held))
	}
	if err := inbox.Refused(t.Context(), held[0], "names no agent", time.Now()); err != nil {
		t.Fatalf("Refused: %v", err)
	}

	waiting, _ := inbox.Claim(t.Context(), "worker-2", time.Minute, 10)
	if len(waiting) != 0 {
		t.Errorf("a refused ask is still claimable: %+v", waiting)
	}
	// And a redelivery of it is still not a new ask: the sender retrying does
	// not get a second chance at an answer the platform already gave.
	fresh, err := inbox.Receive(t.Context(), arrival)
	if err != nil {
		t.Fatalf("Receive after refusal: %v", err)
	}
	if fresh {
		t.Error("a redelivery of a refused ask reported as new")
	}
}

func ask(id string) channel.Arrival {
	return channel.Arrival{
		Channel: "acme-slack", Conversation: "C07-ops", EventID: id,
		Payload: []byte(`{"text":"<@U07BOT> triagem esse chamado"}`),
	}
}

func freshInbox(t *testing.T) *channel.Inbox {
	t.Helper()
	_, pool := channelStore(t)
	if _, err := pool.Exec(t.Context(), `delete from channel_inbox`); err != nil {
		t.Fatalf("clear the inbox: %v", err)
	}
	return channel.NewInbox(pool)
}

/*
Two consumers do not get the same ask.

Listing what is pending and acting on it is not a claim: both open a run and
both reply, and the person is answered twice by an agent that ran twice. The
opener's idempotency key catches the duplicate run and nothing catches the
second reply, because a reply is not a run.
*/
func TestClaim_twoConsumers_doNotGetTheSameAsk(t *testing.T) {
	inbox := freshInbox(t)
	if _, err := inbox.Receive(t.Context(), ask("ev-race")); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	first, err := inbox.Claim(t.Context(), "worker-1", time.Minute, 10)
	if err != nil {
		t.Fatalf("first Claim: %v", err)
	}
	second, err := inbox.Claim(t.Context(), "worker-2", time.Minute, 10)
	if err != nil {
		t.Fatalf("second Claim: %v", err)
	}

	if len(first) != 1 || len(second) != 0 {
		t.Errorf("first got %d and second got %d, want one and none", len(first), len(second))
	}
}

// A consumer that died stops renewing, and the next sweep picks the ask up.
// No reaper to write and none to forget.
func TestClaim_anExpiredLease_isClaimableAgain(t *testing.T) {
	inbox := freshInbox(t)
	if _, err := inbox.Receive(t.Context(), ask("ev-lapsed")); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	if _, err := inbox.Claim(t.Context(), "worker-dead", -time.Minute, 10); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	again, err := inbox.Claim(t.Context(), "worker-2", time.Minute, 10)
	if err != nil {
		t.Fatalf("second Claim: %v", err)
	}
	if len(again) != 1 {
		t.Errorf("claimed = %d, want the lapsed ask back", len(again))
	}
}

/*
A consumer that lost its claim is told, rather than overwriting the answer.

Without the condition, a consumer whose lease expired would write its outcome
over the one the consumer that took over had already recorded; without the
check it would carry on and reply. The reply is the thing: it is what the
person reads, and there is no idempotency key on somebody's attention.
*/
func TestSettle_anAskAlreadySettled_isRefused(t *testing.T) {
	inbox := freshInbox(t)
	arrival := ask("ev-twice")
	if _, err := inbox.Receive(t.Context(), arrival); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	held, err := inbox.Claim(t.Context(), "worker-1", time.Minute, 10)
	if err != nil || len(held) != 1 {
		t.Fatalf("Claim: %v (%d)", err, len(held))
	}
	if err := inbox.Opened(t.Context(), held[0], "run_1", time.Now()); err != nil {
		t.Fatalf("Opened: %v", err)
	}

	if err := inbox.Opened(t.Context(), held[0], "run_2", time.Now()); !errors.Is(err, channel.ErrNotClaimed) {
		t.Errorf("err = %v, want ErrNotClaimed", err)
	}
}

/*
A consumer whose lease lapsed cannot settle the ask somebody else took.

The status alone left this open: a lapsed lease leaves the row pending, so
worker-1 — still running, still holding what it read — would close the ask
worker-2 is working on, and then reply. The lease had a holder and nothing at
the end asked who it was.
*/
func TestSettle_afterAnotherWorkerReclaimedIt_isRefused(t *testing.T) {
	inbox := freshInbox(t)
	if _, err := inbox.Receive(t.Context(), ask("ev-stolen")); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	// Claimed with a lease that is already over, so the next claim takes it.
	lapsed, err := inbox.Claim(t.Context(), "worker-1", -time.Minute, 10)
	if err != nil || len(lapsed) != 1 {
		t.Fatalf("first Claim: %v (%d)", err, len(lapsed))
	}
	if _, err := inbox.Claim(t.Context(), "worker-2", time.Minute, 10); err != nil {
		t.Fatalf("second Claim: %v", err)
	}

	err = inbox.Opened(t.Context(), lapsed[0], "run_late", time.Now())
	if !errors.Is(err, channel.ErrNotClaimed) {
		t.Errorf("err = %v, want the lapsed consumer refused", err)
	}
}
