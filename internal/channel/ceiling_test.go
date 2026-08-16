package channel_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
)

/*
A conversation anybody can mention the bot in is a way for anybody to open runs
(NT-005 §12.2).

Not maliciously — a channel with three hundred people and a helpful agent is
enough. The only thing standing there today is the scope ceiling, which is
shared with everything else in the area: the first busy morning spends the
month and the agents nobody was talking to stop.

So a correspondent has a ceiling of their own. It counts runs rather than
money, because money arrives late — a run's cost is known when it ends, and by
then the next twenty have started.
*/

/*
askingFor sends asks from an account, each one its own delivery.

The counter is the point. A helper that numbered from zero on every call sent
the same delivery twice, the inbox refused the duplicate exactly as it should,
and the test went on to assert about an ask that was never written — which is
how a fixture proves the opposite of what it says.
*/
func askingFor(t *testing.T, p *consumerParts) func(account string, n int) {
	t.Helper()
	sent := 0
	return func(account string, n int) {
		for range n {
			sent++
			a := p.arrival
			a.AskedBy = account
			a.EventID = fmt.Sprintf("%s-%d", account, sent)
			a.Message = fmt.Sprintf("1786.%d", sent)
			if _, err := p.inbox.Receive(t.Context(), a); err != nil {
				t.Fatalf("Receive: %v", err)
			}
		}
	}
}

func TestSweep_pastTheCeiling_theAskIsRefusedAndTheRunIsNotOpened(t *testing.T) {
	c, p := consumerFor(t, "<@U07BOT> triagem olha isso")
	c.WithCeiling(channel.Ceiling{Runs: 2, Per: time.Hour})
	askingFor(t, p)("U9", 2) // three in total: the fixture already wrote one

	opened, err := c.Sweep(t.Context(), time.Minute, 10)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if opened != 2 {
		t.Fatalf("opened = %d, want the ceiling and no more", opened)
	}
	if p.opener.calls != 2 {
		t.Errorf("the opener was called %d times; the refused ask still ran", p.opener.calls)
	}
}

/*
The refusal is said once, however many times somebody asks.

A limit that answers every message it rejects is a limit that amplifies the
flood: the person who sent fifty gets fifty replies, the conversation is
unreadable, and it is our bot that made it so. The rest are recorded — an
operator asked "why did nothing happen" can still read them — and not said.
*/
func TestSweep_aFloodPastTheCeiling_isSaidOnce(t *testing.T) {
	c, p := consumerFor(t, "<@U07BOT> triagem olha isso")
	c.WithCeiling(channel.Ceiling{Runs: 1, Per: time.Hour})
	askingFor(t, p)("U9", 5)

	if _, err := c.Sweep(t.Context(), time.Minute, 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	said, err := c.Answer(t.Context(), time.Minute, 10)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if said != 1 {
		t.Fatalf("said %d refusals, want one however many were sent", said)
	}
	if len(p.answers.said) != 1 || !strings.Contains(p.answers.said[0], "too many") {
		t.Errorf("said %q, want a sentence naming the limit", p.answers.said)
	}
}

// The ceiling is one person's, not the conversation's. A busy colleague must
// not spend the limit of everybody standing next to them.
func TestSweep_theCeiling_isOneCorrespondentsAndNotTheConversations(t *testing.T) {
	c, p := consumerFor(t, "<@U07BOT> triagem olha isso")
	ask := askingFor(t, p)
	c.WithCeiling(channel.Ceiling{Runs: 1, Per: time.Hour})
	ask("U9", 3)
	ask("U42", 1)

	opened, err := c.Sweep(t.Context(), time.Minute, 20)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	// One for the fixture's own ask, and one for the second person. U9's
	// extras are refused; U42 has spent nothing.
	if opened != 2 {
		t.Fatalf("opened = %d, want one each", opened)
	}
}

/*
The window slides, or a ceiling is a ban.

Counted from a clock the test owns, like every other time in this platform:
`time.Now` inside the decision would make this assertion a race against the
hour it happens to run in.
*/
func TestSweep_pastTheWindow_theCorrespondentAsksAgain(t *testing.T) {
	c, p := consumerFor(t, "<@U07BOT> triagem olha isso")
	ask := askingFor(t, p)
	at := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	c.WithCeiling(channel.Ceiling{Runs: 1, Per: time.Hour}).WithClock(func() time.Time { return at })

	if _, err := c.Sweep(t.Context(), time.Minute, 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	ask("U9", 1)

	// Still inside the hour: refused.
	at = at.Add(30 * time.Minute)
	if opened, err := c.Sweep(t.Context(), time.Minute, 10); err != nil || opened != 0 {
		t.Fatalf("opened %d (%v) inside the window", opened, err)
	}

	// And out the other side of it.
	at = at.Add(90 * time.Minute)
	ask("U9", 1)
	opened, err := c.Sweep(t.Context(), time.Minute, 10)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if opened != 1 {
		t.Errorf("opened = %d after the window passed; the ceiling became a ban", opened)
	}
}

/*
Two workers, one slot: the ceiling is decided atomically or it is decoration.

Counting and then opening is a check followed by a decision, and between them
is every other worker. Both see nobody has spent anything, both open, and a
ceiling of one lets through as many runs as there are processes sweeping. It is
worst exactly when it matters — a flood is what puts several asks in the inbox
at once, which is what gives two workers something to claim.

Deterministic rather than a race the test hopes to lose: the first worker holds
its claim without settling, which is the state the window exists in, and the
second decides while it is held.
*/
func TestSweep_twoWorkersOverOneSlot_onlyOneRunOpens(t *testing.T) {
	first, p := consumerFor(t, "<@U07BOT> triagem olha isso")
	first.WithCeiling(channel.Ceiling{Runs: 1, Per: time.Hour})
	askingFor(t, p)("U9", 1)

	// The first worker takes its ask and gets the slot, and has not settled:
	// the run is open and the inbox still says pending.
	held, err := p.inbox.Claim(t.Context(), "worker-1", time.Minute, 1)
	if err != nil || len(held) != 1 {
		t.Fatalf("Claim: %v (%d)", err, len(held))
	}
	if _, granted, err := p.inbox.Reserve(t.Context(), held[0],
		channel.Quota{Runs: 1, Since: time.Now().Add(-time.Hour)}); err != nil || !granted {
		t.Fatalf("the first worker was not granted the only slot: %v (%v)", granted, err)
	}

	// The second worker decides while that claim is still held.
	second := consumerLike(t, p, "worker-2")
	second.WithCeiling(channel.Ceiling{Runs: 1, Per: time.Hour})
	opened, err := second.Sweep(t.Context(), time.Minute, 10)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if opened != 0 {
		t.Fatalf("opened %d; the slot was already reserved and both workers spent it", opened)
	}
}

// A slot held by a worker that died is not held for ever. It goes with the
// lease, so the correspondent waits a lease and not a window — otherwise
// twenty crashes are an hour of silence for somebody who did nothing wrong.
func TestReserve_aSlotHeldByAWorkerThatDied_isFreedWithItsLease(t *testing.T) {
	_, p := consumerFor(t, "<@U07BOT> triagem olha isso")
	askingFor(t, p)("U9", 1)

	// A lease already in the past, which is what a dead worker leaves behind.
	// Written as a negative one rather than a short one and a sleep: the state
	// under test is "lapsed", and waiting for it is a slower way to be flaky.
	dead, err := p.inbox.Claim(t.Context(), "gone", -time.Second, 1)
	if err != nil || len(dead) != 1 {
		t.Fatalf("Claim: %v (%d)", err, len(dead))
	}
	quota := channel.Quota{Runs: 1, Since: time.Now().Add(-time.Hour)}
	if _, granted, err := p.inbox.Reserve(t.Context(), dead[0], quota); err != nil || !granted {
		t.Fatalf("reserve: %v (%v)", granted, err)
	}

	// Its lease has already lapsed. Whoever picks the ask up next gets the
	// slot back rather than meeting a reservation nobody is using.
	next, err := p.inbox.Claim(t.Context(), "worker-2", time.Minute, 1)
	if err != nil || len(next) != 1 {
		t.Fatalf("reclaim: %v (%d)", err, len(next))
	}
	spent, granted, err := p.inbox.Reserve(t.Context(), next[0], quota)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if !granted {
		t.Errorf("refused with %d spent; the dead worker's slot was never given back", spent)
	}
}

/*
The claim hands back what it was asked for and no more.

A sweep configured for twenty that drains the whole inbox holds every one of
those asks under one lease, and a process that dies loses the lot for a lease
rather than the twenty it was told to take.
*/
func TestClaim_moreWaitingThanTheLimit_handsBackTheLimit(t *testing.T) {
	inbox := freshInbox(t)
	for _, id := range []string{"lim-1", "lim-2", "lim-3"} {
		if _, err := inbox.Receive(t.Context(), ask(id)); err != nil {
			t.Fatalf("Receive: %v", err)
		}
	}

	held, err := inbox.Claim(t.Context(), "worker-1", time.Minute, 1)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(held) != 1 {
		t.Fatalf("claimed %d with a limit of one", len(held))
	}
}
