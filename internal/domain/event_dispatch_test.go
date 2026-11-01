package domain_test

import (
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

var lagNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func TestEventDispatchBacklog_lag_isHowLongTheOldestRunPastTheCursorHasWaited(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		backlog domain.EventDispatchBacklog
		want    time.Duration
	}{
		{"caught up", domain.EventDispatchBacklog{}, 0},
		{"one waiting", domain.EventDispatchBacklog{Ready: 1, OldestFinishedAt: lagNow.Add(-90 * time.Second)}, 90 * time.Second},
		{"held only", domain.EventDispatchBacklog{HeldByXmin: 3, OldestFinishedAt: lagNow.Add(-time.Hour)}, time.Hour},
		// A worker stamps ended_at with its own clock; one ahead of ours is
		// not a run from the future waiting a negative time.
		{"stamped ahead", domain.EventDispatchBacklog{Ready: 1, OldestFinishedAt: lagNow.Add(time.Second)}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.backlog.Lag(lagNow); got != c.want {
				t.Fatalf("Lag = %v, want %v", got, c.want)
			}
		})
	}
}

func TestEventDispatchBacklog_status_degradesOnlyPastTheThreshold(t *testing.T) {
	t.Parallel()
	at := func(waited time.Duration) domain.EventDispatchBacklog {
		return domain.EventDispatchBacklog{Ready: 1, OldestFinishedAt: lagNow.Add(-waited)}
	}
	cases := []struct {
		name    string
		backlog domain.EventDispatchBacklog
		want    string
	}{
		{"caught up", domain.EventDispatchBacklog{}, domain.EventDispatchOK},
		{"at the threshold", at(domain.EventDispatchDegradedAfter), domain.EventDispatchOK},
		{"past the threshold", at(domain.EventDispatchDegradedAfter + time.Second), domain.EventDispatchDegraded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.backlog.Status(lagNow); got != c.want {
				t.Fatalf("Status = %q, want %q", got, c.want)
			}
		})
	}
}

func TestEventDispatchBacklog_xactAge_isUnknownWhenTheHolderIsHidden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		xact      domain.OpenXact
		want      time.Duration
		wantKnown bool
	}{
		{"none open", domain.OpenXact{}, 0, true},
		{"visible", domain.OpenXact{Open: true, StartedAt: lagNow.Add(-10 * time.Minute)}, 10 * time.Minute, true},
		{"hidden from the role", domain.OpenXact{Open: true}, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, known := domain.EventDispatchBacklog{Xact: c.xact}.XactAge(lagNow)
			if got != c.want || known != c.wantKnown {
				t.Fatalf("XactAge = %v, %v; want %v, %v", got, known, c.want, c.wantKnown)
			}
		})
	}
}

func TestEventDispatchBacklog_held_isWhetherCommittedRunsWaitOnXmin(t *testing.T) {
	t.Parallel()
	if (domain.EventDispatchBacklog{Ready: 4}).Held() {
		t.Fatal("runs the feed can read are not held")
	}
	if !(domain.EventDispatchBacklog{HeldByXmin: 1}).Held() {
		t.Fatal("a run committed past xmin is held")
	}
}
