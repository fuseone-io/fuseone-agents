package trigger_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

/*
The backlog watch (#228) reads what the dispatcher has not reached and hands
it, with the instant it was read by the injected clock, to whatever records
it. A read that fails is recorded as a failure rather than as an empty
backlog: nothing waiting and nobody looked are different facts.
*/

type backlogSink struct {
	got        []domain.EventDispatchBacklog
	at         []time.Time
	unobserved int
}

func (s *backlogSink) EventDispatch(b domain.EventDispatchBacklog, now time.Time) {
	s.got = append(s.got, b)
	s.at = append(s.at, now)
}

func (s *backlogSink) EventDispatchUnobserved() { s.unobserved++ }

func TestBacklogWatch_recordsWhatTheSweepHasNotReached_atTheClocksInstant(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	finishSources(t, store, 0, 3)
	clock := fixedClock{t: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	sink := &backlogSink{}
	watch := trigger.NewBacklogWatch(store, sink, clock)

	if err := watch.Observe(context.Background()); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if _, err := chainDispatcher(store).Sweep(context.Background(), 30); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if err := watch.Observe(context.Background()); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	if len(sink.got) != 2 || sink.got[0].Ready != 3 || sink.got[1].Ready != 0 {
		t.Fatalf("observed %+v; want three waiting, then none after the sweep", sink.got)
	}
	if !sink.at[0].Equal(clock.t) {
		t.Fatalf("recorded at %v, want the clock's %v", sink.at[0], clock.t)
	}
}

type failingBacklog struct{}

func (failingBacklog) EventDispatchBacklog(context.Context) (domain.EventDispatchBacklog, error) {
	return domain.EventDispatchBacklog{}, errors.New("connection refused")
}

func TestBacklogWatch_aFailedRead_isCountedNotRecordedAsEmpty(t *testing.T) {
	t.Parallel()
	sink := &backlogSink{}
	watch := trigger.NewBacklogWatch(failingBacklog{}, sink, fixedClock{t: time.Now()})

	if err := watch.Observe(context.Background()); err == nil {
		t.Fatal("Observe swallowed the failed read")
	}
	if sink.unobserved != 1 || len(sink.got) != 0 {
		t.Fatalf("unobserved %d, recorded %+v; want the failure counted and nothing recorded", sink.unobserved, sink.got)
	}
}
