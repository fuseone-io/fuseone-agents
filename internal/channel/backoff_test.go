package channel_test

import (
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
)

/*
A run that could not be announced waits before it is tried again.

Kept in the sweep and never retired — which is right, because the day somebody
configures the connection the run is still there to be told about — it came back
every thirty seconds until the window closed. Up to 2,880 attempts written per
run, against a table nobody is reading, for a run nothing was going to reach.

Two schedules, because two failures are not alike. A destination refusing is an
incident and somebody is probably fixing it. Nothing configured to hear the run
at all is an installation part-way through being set up, and the fix is a person
doing something rather than a network recovering.
*/
func TestUnreported_afterAFailedAttempt_waitsBeforeTryingAgain(t *testing.T) {
	for _, one := range []struct {
		name  string
		code  string
		after time.Duration
		tried bool
	}{
		{"a destination that refused, moments ago", channel.CodeDeliveryFailed,
			10 * time.Second, false},
		{"a destination that refused, a while ago", channel.CodeDeliveryFailed,
			5 * time.Minute, true},
		// Longer, deliberately: the same run coming back twice a minute writes
		// a day of attempts about an installation nobody has finished
		// configuring.
		{"nothing configured, minutes ago", channel.CodeNowhereToSayIt,
			5 * time.Minute, false},
		{"nothing configured, an hour ago", channel.CodeNowhereToSayIt,
			time.Hour, true},
	} {
		t.Run(one.name, func(t *testing.T) {
			store, pool := channelStore(t)

			awaitApproval(t, pool, "run-waiting")
			pending, err := store.Unreported(t.Context(), time.Now().Add(-channel.Window), 50)
			if err != nil {
				t.Fatalf("unreported: %v", err)
			}
			if len(pending) != 1 {
				t.Fatalf("pending = %+v, want the parked run", pending)
			}

			if err := store.RecordFailure(t.Context(), channel.DeliveryFailure{
				Announcement: channel.Announcement{
					RunID: "run-waiting", Event: channel.EventParked,
					AtSeq: pending[0].AtSeq,
				},
				Code: one.code, Scope: domain.Scope{Company: "acme", Area: "ops"},
				SeenAt: time.Now().Add(-one.after),
			}); err != nil {
				t.Fatalf("record the failure: %v", err)
			}

			again, err := store.Unreported(t.Context(), time.Now().Add(-channel.Window), 50)
			if err != nil {
				t.Fatalf("unreported after the failure: %v", err)
			}
			if tried := len(again) == 1; tried != one.tried {
				t.Fatalf("tried again = %v, want %v (%s after a %s)",
					tried, one.tried, one.after, one.code)
			}
		})
	}
}

/*
And the wait grows, so a destination nobody is fixing is asked about less often.

Doubling from a floor to a ceiling. The whole schedule sits inside the window,
so a run is retried many times before it leaves: waiting longer is never giving
up, which is the one thing this must not become.
*/
func TestUnreported_repeatedFailures_waitLonger(t *testing.T) {
	store, pool := channelStore(t)

	awaitApproval(t, pool, "run-patient")
	pending, err := store.Unreported(t.Context(), time.Now().Add(-channel.Window), 50)
	if err != nil || len(pending) != 1 {
		t.Fatalf("unreported = %+v, %v", pending, err)
	}

	// Five attempts, the last of them four minutes ago. One attempt would have
	// been tried again by now; five are not.
	failure := channel.DeliveryFailure{
		Announcement: channel.Announcement{
			RunID: "run-patient", Event: channel.EventParked, AtSeq: pending[0].AtSeq,
		},
		Code: channel.CodeDeliveryFailed, Scope: domain.Scope{Company: "acme", Area: "ops"},
	}
	for i := range 5 {
		failure.SeenAt = time.Now().Add(-4 * time.Minute).Add(time.Duration(i) * time.Second)
		if err := store.RecordFailure(t.Context(), failure); err != nil {
			t.Fatalf("record attempt %d: %v", i, err)
		}
	}

	again, err := store.Unreported(t.Context(), time.Now().Add(-channel.Window), 50)
	if err != nil {
		t.Fatalf("unreported: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("tried again after five failures and four minutes: %+v", again)
	}
}

/*
How long to wait is decided by what went wrong last time.

Read over every failure ever recorded about one announcement, the schedule was a
history rather than a state. An installation configured this morning carries a
handful of "nothing is configured" attempts; the first real refusal after that
then inherited their count and their patience — half an hour before the next
try, for a problem somebody may be fixing right now.

Both transitions, because either direction is a run waiting for the wrong
reason.
*/
func TestUnreported_theScheduleFollowsTheLatestFailure(t *testing.T) {
	for _, one := range []struct {
		name    string
		history string
		latest  string
		after   time.Duration
		tried   bool
	}{
		{
			// Six attempts at nothing configured, then somebody configures a
			// connection and the destination refuses. The incident schedule
			// starts over: a minute, not the half hour six attempts had earned.
			name:    "an incident after an unconfigured history",
			history: channel.CodeNowhereToSayIt, latest: channel.CodeDeliveryFailed,
			after: 2 * time.Minute, tried: true,
		},
		{
			// And the other way: a destination that used to refuse, and now
			// there is nothing configured at all. That waits the longer time
			// from the start rather than inheriting a minute.
			name:    "nothing configured after an incident history",
			history: channel.CodeDeliveryFailed, latest: channel.CodeNowhereToSayIt,
			after: 2 * time.Minute, tried: false,
		},
	} {
		t.Run(one.name, func(t *testing.T) {
			store, pool := channelStore(t)

			awaitApproval(t, pool, "run-history")
			pending, err := store.Unreported(t.Context(), time.Now().Add(-channel.Window), 50)
			if err != nil || len(pending) != 1 {
				t.Fatalf("unreported = %+v, %v", pending, err)
			}

			failure := channel.DeliveryFailure{
				Announcement: channel.Announcement{
					RunID: "run-history", Event: channel.EventParked,
					AtSeq: pending[0].AtSeq,
				},
				Scope: domain.Scope{Company: "acme", Area: "ops"},
			}
			// Six of the old kind, an hour ago.
			failure.Code = one.history
			for i := range 6 {
				failure.SeenAt = time.Now().Add(-time.Hour).Add(time.Duration(i) * time.Second)
				if err := store.RecordFailure(t.Context(), failure); err != nil {
					t.Fatalf("record the history: %v", err)
				}
			}
			// One of the new kind, just now.
			failure.Code, failure.SeenAt = one.latest, time.Now().Add(-one.after)
			if err := store.RecordFailure(t.Context(), failure); err != nil {
				t.Fatalf("record the latest failure: %v", err)
			}

			again, err := store.Unreported(t.Context(), time.Now().Add(-channel.Window), 50)
			if err != nil {
				t.Fatalf("unreported: %v", err)
			}
			if tried := len(again) == 1; tried != one.tried {
				t.Fatalf("tried again = %v, want %v: the schedule follows %s",
					tried, one.tried, one.latest)
			}
		})
	}
}

/*
And an intervening cause does not give the old one a fresh start.

The count belongs to the cause, and a cause is one row: a destination and a
normalised operational code. So a destination that refused six times, was
unreachable for a while for another reason, and now refuses again is refusing
for the seventh time — not the first. Nothing about the other reason having
happened in between makes the first six untrue.

Both directions, and each of them twice: once shortly after the return, to say
the count was inherited, and once past the ceiling, to say the inherited wait is
still bounded by it. An inherited count that kept doubling would be how a run
quietly leaves the window unannounced, which is the failure the whole schedule
exists inside.
*/
func TestUnreported_aCauseReturningAfterAnother_keepsItsOwnCount(t *testing.T) {
	for _, one := range []struct {
		name    string
		cause   string
		between string
		before  int
		probe   time.Duration
		tried   bool
	}{
		{
			// Seven refusals is 64 minutes, and a first refusal would be a
			// minute. Ten minutes after the return tells the two apart.
			name:  "an incident that returns after an unconfigured spell",
			cause: channel.CodeDeliveryFailed, between: channel.CodeNowhereToSayIt,
			before: 6, probe: 10 * time.Minute, tried: false,
		},
		{
			// Eleven refusals doubles to 128 minutes, and the ceiling is 120.
			name:  "an incident that returns often enough to reach the ceiling",
			cause: channel.CodeDeliveryFailed, between: channel.CodeNowhereToSayIt,
			before: 10, probe: 2*time.Hour + 5*time.Minute, tried: true,
		},
		{
			// The other direction: nothing configured, then an incident, then
			// nothing configured again. Seven attempts on the slower schedule
			// is already at its six-hour ceiling.
			name:  "an unconfigured installation after an incident",
			cause: channel.CodeNowhereToSayIt, between: channel.CodeDeliveryFailed,
			before: 6, probe: 2 * time.Hour, tried: false,
		},
		{
			name:  "an unconfigured installation past its ceiling",
			cause: channel.CodeNowhereToSayIt, between: channel.CodeDeliveryFailed,
			before: 10, probe: 6*time.Hour + 5*time.Minute, tried: true,
		},
	} {
		t.Run(one.name, func(t *testing.T) {
			store, pool := channelStore(t)

			awaitApproval(t, pool, "run-returning")
			pending, err := store.Unreported(t.Context(), time.Now().Add(-channel.Window), 50)
			if err != nil || len(pending) != 1 {
				t.Fatalf("unreported = %+v, %v", pending, err)
			}

			failure := channel.DeliveryFailure{
				Announcement: channel.Announcement{
					RunID: "run-returning", Event: channel.EventParked,
					AtSeq: pending[0].AtSeq,
				},
				Scope: domain.Scope{Company: "acme", Area: "ops"},
			}
			record := func(code string, at time.Time) {
				t.Helper()
				failure.Code, failure.SeenAt = code, at
				if err := store.RecordFailure(t.Context(), failure); err != nil {
					t.Fatalf("record %s: %v", code, err)
				}
			}

			for i := range one.before {
				record(one.cause, time.Now().Add(-20*time.Hour).Add(time.Duration(i)*time.Second))
			}
			record(one.between, time.Now().Add(-15*time.Hour))
			record(one.cause, time.Now().Add(-one.probe))

			again, err := store.Unreported(t.Context(), time.Now().Add(-channel.Window), 50)
			if err != nil {
				t.Fatalf("unreported: %v", err)
			}
			if tried := len(again) == 1; tried != one.tried {
				t.Fatalf("tried again = %v, want %v: %s returned for the %d%s time",
					tried, one.tried, one.cause, one.before+1, "th")
			}
		})
	}
}
