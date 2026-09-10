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
